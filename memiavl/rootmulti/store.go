package rootmulti

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tidwall/wal"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/store/listenkv"
	"cosmossdk.io/store/mem"
	"cosmossdk.io/store/metrics"
	pruningtypes "cosmossdk.io/store/pruning/types"
	"cosmossdk.io/store/rootmulti"
	"cosmossdk.io/store/transient"
	"cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/initia-labs/store/memiavl"
	"github.com/initia-labs/store/memiavl/cachemulti"
)

const CommitInfoFileName = "commit_infos"

var (
	_ types.CommitMultiStore = (*Store)(nil)
	_ types.Queryable        = (*Store)(nil)
)

type Store struct {
	dir    string
	db     *memiavl.DB
	logger log.Logger

	// to keep it comptaible with cosmos-sdk 0.46, merge the memstores into commit info
	lastCommitInfo *types.CommitInfo

	storesParams map[types.StoreKey]storeParams
	keysByName   map[string]types.StoreKey
	stores       map[types.StoreKey]types.CommitStore
	listeners    map[types.StoreKey]*types.MemoryListener

	supportExportNonSnapshotVersion bool

	opts memiavl.Options

	queryCache *queryDBCache
}

func NewStore(dir string, logger log.Logger, supportExportNonSnapshotVersion bool) *Store {
	store := &Store{
		dir:    dir,
		logger: logger,

		storesParams: make(map[types.StoreKey]storeParams),
		keysByName:   make(map[string]types.StoreKey),
		stores:       make(map[types.StoreKey]types.CommitStore),
		listeners:    make(map[types.StoreKey]*types.MemoryListener),

		supportExportNonSnapshotVersion: supportExportNonSnapshotVersion,
	}
	store.queryCache = newQueryDBCache(store)
	return store
}

// flush writes all the pending change sets to memiavl tree.
func (rs *Store) flush() error {
	var changeSets []*memiavl.NamedChangeSet
	for key := range rs.stores {
		// it'll unwrap the inter-block cache
		store := rs.GetCommitStore(key)
		if memiavlStore, ok := store.(*memiavl.Store); ok {
			cs := memiavlStore.PopChangeSet()
			if len(cs.Pairs) > 0 {
				changeSets = append(changeSets, &memiavl.NamedChangeSet{
					Name:      key.Name(),
					Changeset: cs,
				})
			}
		}
	}
	sort.SliceStable(changeSets, func(i, j int) bool {
		return changeSets[i].Name < changeSets[j].Name
	})

	return rs.db.ApplyChangeSets(changeSets)
}

// WorkingHash returns the app hash of the working tree,
//
// Implements interface Committer.
func (rs *Store) WorkingHash() []byte {
	if err := rs.flush(); err != nil {
		panic(err)
	}
	commitInfo := convertCommitInfo(rs.db.WorkingCommitInfo())
	return commitInfo.Hash()
}

// Commit Implements interface Committer
func (rs *Store) Commit() types.CommitID {
	if err := rs.flush(); err != nil {
		panic(err)
	}

	for _, store := range rs.stores {
		if store.GetStoreType() != types.StoreTypeIAVL {
			_ = store.Commit()
		}
	}

	_, err := rs.db.Commit()
	if err != nil {
		panic(err)
	}

	// the underlying memiavl tree might be reloaded, update the tree.
	for key := range rs.stores {
		store := rs.stores[key]
		if store.GetStoreType() == types.StoreTypeIAVL {
			store.(*memiavl.Store).SetTree(rs.db.TreeByName(key.Name()))
		}
	}

	rs.lastCommitInfo = convertCommitInfo(rs.db.LastCommitInfo())

	// add the committed version into historical query cache
	rs.queryCache.AddHistoricalVersion(rs.db, rs.lastCommitInfo.Version)

	return rs.lastCommitInfo.CommitID()
}

func (rs *Store) Close() error {
	rs.queryCache.Close()
	return rs.db.Close()
}

// LastCommitID Implements interface Committer
func (rs *Store) LastCommitID() types.CommitID {
	if rs.lastCommitInfo == nil {
		v, err := memiavl.GetLatestVersion(rs.dir)
		if err != nil {
			panic(fmt.Errorf("failed to get latest version: %w", err))
		}
		return types.CommitID{Version: v}
	}

	return rs.lastCommitInfo.CommitID()
}

// SetPruning Implements interface Committer
func (rs *Store) SetPruning(pruningtypes.PruningOptions) {
}

// SetMetrics sets the metrics gatherer for the store package
func (rs *Store) SetMetrics(metrics metrics.StoreMetrics) {
}

// GetPruning Implements interface Committer
func (rs *Store) GetPruning() pruningtypes.PruningOptions {
	return pruningtypes.NewPruningOptions(pruningtypes.PruningDefault)
}

// GetStoreType Implements interface Store
func (rs *Store) GetStoreType() types.StoreType {
	return types.StoreTypeMulti
}

// CacheWrap Implements interface CacheWrapper
func (rs *Store) CacheWrap() types.CacheWrap {
	return rs.CacheMultiStore().(types.CacheWrap)
}

// CacheWrapWithTrace Implements interface CacheWrapper
func (rs *Store) CacheWrapWithTrace(_ io.Writer, _ types.TraceContext) types.CacheWrap {
	return rs.CacheWrap()
}

// CacheMultiStore Implements interface MultiStore
func (rs *Store) CacheMultiStore() types.CacheMultiStore {
	stores := make(map[types.StoreKey]types.CacheWrapper)
	for k, v := range rs.stores {
		store := types.CacheWrapper(v)
		if kv, ok := store.(types.KVStore); ok {
			// Wire the listenkv.Store to allow listeners to observe the writes from the cache store,
			// set same listeners on cache store will observe duplicated writes.
			if rs.ListeningEnabled(k) {
				store = listenkv.NewStore(kv, k, rs.listeners[k])
			}
		}
		stores[k] = store
	}
	return cachemulti.NewStore(stores, nil, nil, nil)
}

// CacheMultiStoreWithVersion Implements interface MultiStore
// used to createQueryContext, abci_query or grpc query service.
func (rs *Store) CacheMultiStoreWithVersion(version int64) (types.CacheMultiStore, error) {
	latestVersion := rs.LatestVersion()
	if version > latestVersion {
		return nil, sdkerrors.ErrInvalidHeight.Wrapf("requested version %d is greater than latest version %d", version, latestVersion)
	}
	if version <= 0 {
		version = latestVersion
	}

	cloned, ok := rs.queryCache.GetHistoricalTrees(version)
	if !ok {
		return nil, sdkerrors.ErrInvalidHeight.Wrapf("historical version not ready: %d", version)
	}

	stores := make(map[types.StoreKey]types.CacheWrapper)

	// add the transient/mem stores registered in current app.
	for k, store := range rs.stores {
		if store.GetStoreType() != types.StoreTypeIAVL {
			stores[k] = store
		}
	}

	// add all the iavl stores at the target version.
	for _, nt := range cloned.Trees() {
		key, ok := rs.keysByName[nt.Name]
		if !ok {
			_ = cloned.Close()
			return nil, fmt.Errorf("unknown store key: %s", nt.Name)
		}
		stores[key] = memiavl.NewStore(nt.Tree, rs.logger)
	}

	return cachemulti.NewStore(stores, nil, nil, cloned), nil
}

// GetStore Implements interface MultiStore
func (rs *Store) GetStore(key types.StoreKey) types.Store {
	s, ok := rs.stores[key]
	if !ok {
		panic(fmt.Sprintf("store does not exist for key: %s", key.Name()))
	}
	return s
}

// GetKVStore Implements interface MultiStore
func (rs *Store) GetKVStore(key types.StoreKey) types.KVStore {
	s, ok := rs.GetStore(key).(types.KVStore)
	if !ok {
		panic(fmt.Sprintf("store with key %v is not KVStore", key))
	}
	return s
}

// TracingEnabled Implements interface MultiStore
func (rs *Store) TracingEnabled() bool {
	return false
}

// SetTracer Implements interface MultiStore
func (rs *Store) SetTracer(w io.Writer) types.MultiStore {
	return nil
}

// SetTracingContext Implements interface MultiStore
func (rs *Store) SetTracingContext(types.TraceContext) types.MultiStore {
	return nil
}

// LatestVersion Implements interface MultiStore
func (rs *Store) LatestVersion() int64 {
	return rs.db.Version()
}

// MountStoreWithDB Implements interface CommitMultiStore
func (rs *Store) MountStoreWithDB(key types.StoreKey, typ types.StoreType, _ dbm.DB) {
	if key == nil {
		panic("MountIAVLStore() key cannot be nil")
	}
	if _, ok := rs.storesParams[key]; ok {
		panic(fmt.Sprintf("store duplicate store key %v", key))
	}
	if _, ok := rs.keysByName[key.Name()]; ok {
		panic(fmt.Sprintf("store duplicate store key name %v", key))
	}
	rs.storesParams[key] = newStoreParams(key, typ)
	rs.keysByName[key.Name()] = key
}

// GetCommitStore Implements interface CommitMultiStore
func (rs *Store) GetCommitStore(key types.StoreKey) types.CommitStore {
	return rs.stores[key]
}

// GetCommitKVStore Implements interface CommitMultiStore
func (rs *Store) GetCommitKVStore(key types.StoreKey) types.CommitKVStore {
	store, ok := rs.GetCommitStore(key).(types.CommitKVStore)
	if !ok {
		panic(fmt.Sprintf("store with key %v is not CommitKVStore", key))
	}

	return store
}

// LoadLatestVersion Implements interface CommitMultiStore
// used by normal node startup.
func (rs *Store) LoadLatestVersion() error {
	return rs.LoadVersionAndUpgrade(0, nil)
}

// LoadLatestVersionAndUpgrade Implements interface CommitMultiStore
func (rs *Store) LoadLatestVersionAndUpgrade(upgrades *types.StoreUpgrades) error {
	return rs.LoadVersionAndUpgrade(0, upgrades)
}

// LoadVersionAndUpgrade Implements interface CommitMultiStore
// used by node startup with UpgradeStoreLoader
func (rs *Store) LoadVersionAndUpgrade(version int64, upgrades *types.StoreUpgrades) error {
	rs.queryCache.Reset()

	storesKeys := make([]types.StoreKey, 0, len(rs.storesParams))
	for key := range rs.storesParams {
		storesKeys = append(storesKeys, key)
	}
	// deterministic iteration order for upgrades
	sort.Slice(storesKeys, func(i, j int) bool {
		return storesKeys[i].Name() < storesKeys[j].Name()
	})

	initialStores := make([]string, 0, len(storesKeys))
	for _, key := range storesKeys {
		if rs.storesParams[key].typ == types.StoreTypeIAVL {
			initialStores = append(initialStores, key.Name())
		}
	}

	opts := rs.opts
	opts.CreateIfMissing = true
	opts.InitialStores = initialStores
	opts.TargetVersion = uint64(version)
	db, err := memiavl.Load(rs.dir, opts)
	if err != nil {
		return errors.Wrapf(err, "fail to load memiavl at %s", rs.dir)
	}

	// read and set initial version from metadata
	metadata, err := memiavl.ReadMetadata(rs.dir)
	if err != nil {
		return errors.Wrapf(err, "fail to read memiavl metadata at %s", rs.dir)
	}
	rs.opts.InitialVersion = uint64(metadata.InitialVersion)

	var treeUpgrades []*memiavl.TreeNameUpgrade
	if upgrades != nil {
		for _, name := range upgrades.Deleted {
			treeUpgrades = append(treeUpgrades, &memiavl.TreeNameUpgrade{Name: name, Delete: true})
		}
		for _, name := range upgrades.Added {
			treeUpgrades = append(treeUpgrades, &memiavl.TreeNameUpgrade{Name: name})
		}
		for _, rename := range upgrades.Renamed {
			treeUpgrades = append(treeUpgrades, &memiavl.TreeNameUpgrade{Name: rename.NewKey, RenameFrom: rename.OldKey})
		}
	}

	if len(treeUpgrades) > 0 {
		if err := db.ApplyUpgrades(treeUpgrades); err != nil {
			return err
		}
	}

	newStores := make(map[types.StoreKey]types.CommitStore, len(storesKeys))
	for _, key := range storesKeys {
		newStores[key], err = rs.loadCommitStoreFromParams(db, key, rs.storesParams[key])
		if err != nil {
			return err
		}
	}

	rs.db = db
	rs.stores = newStores
	// to keep the root hash compatible with cosmos-sdk 0.46
	if db.Version() != 0 {
		rs.lastCommitInfo = convertCommitInfo(db.LastCommitInfo())
	} else {
		rs.lastCommitInfo = &types.CommitInfo{}
	}
	if rs.lastCommitInfo.Version > 0 {
		rs.queryCache.AddHistoricalVersion(rs.db, rs.lastCommitInfo.Version)
		go rs.loadQueryCache(rs.lastCommitInfo.Version)
	}

	return nil
}

// loadQueryCache loads the historical query cache db in background.
func (rs *Store) loadQueryCache(latestVersion int64) {
	opts := rs.opts
	opts.ReadOnly = true
	cacheLimit := rs.queryCache.CacheLimit()
	if cacheLimit <= 0 {
		return
	}

	startVersion := uint64(1)
	if latestVersion > int64(cacheLimit) {
		startVersion = uint64(latestVersion - int64(cacheLimit) + 1)
	}

	// if initial version is greater than 1, this means initial version is set
	// snapshot height + 1, so we need to set the target version accordingly.
	if rs.opts.InitialVersion > 1 {
		initialHeight := rs.opts.InitialVersion - 1
		if startVersion < max(initialHeight, 1) {
			startVersion = max(initialHeight, 1)
		}
	}

	rs.logger.Info("loading memiavl query cache", "from", startVersion, "to", latestVersion)

	opts.TargetVersion = startVersion
	db, err := memiavl.Load(rs.dir, opts)
	if err != nil {
		rs.logger.Error("failed to load memiavl for query cache", "error", err)
		return
	}
	defer db.Close()
	wal, err := memiavl.OpenWAL(filepath.Join(rs.dir, "wal"), &wal.Options{NoCopy: true, NoSync: true})
	if err != nil {
		rs.logger.Error("failed to open memiavl wal for query cache", "error", err)
		return
	}
	defer wal.Close()

	// add the initial historical version into query cache
	rs.queryCache.AddHistoricalVersion(db, int64(startVersion))

	// catch up the  WAL logs to the latest version and add historical versions into query cache
	for h := startVersion + 1; h < uint64(latestVersion); h++ {
		if err = db.MultiTree.CatchupWAL(wal, int64(h)); err != nil {
			rs.logger.Error("failed to catchup memiavl wal for query cache", "height", h, "error", err)
			return
		}
		rs.queryCache.AddHistoricalVersion(db, int64(h))
	}

	rs.logger.Info("finished loading memiavl query cache", "from", startVersion, "to", latestVersion)
}

func (rs *Store) loadCommitStoreFromParams(db *memiavl.DB, key types.StoreKey, params storeParams) (types.CommitStore, error) {
	switch params.typ {
	case types.StoreTypeMulti:
		panic("recursive MultiStores not yet supported")
	case types.StoreTypeIAVL:
		tree := db.TreeByName(key.Name())
		if tree == nil {
			return nil, fmt.Errorf("new store is not added in upgrades: %s", key.Name())
		}
		return types.CommitStore(memiavl.NewStore(tree, rs.logger)), nil
	case types.StoreTypeDB:
		panic("recursive MultiStores not yet supported")
	case types.StoreTypeTransient:
		if _, ok := key.(*types.TransientStoreKey); !ok {
			return nil, fmt.Errorf("unexpected key type for a TransientStoreKey; got: %s, %T", key.String(), key)
		}

		return transient.NewStore(), nil

	case types.StoreTypeMemory:
		if _, ok := key.(*types.MemoryStoreKey); !ok {
			return nil, fmt.Errorf("unexpected key type for a MemoryStoreKey; got: %s", key.String())
		}

		return mem.NewStore(), nil

	default:
		panic(fmt.Sprintf("unrecognized store type %v", params.typ))
	}
}

// LoadVersion Implements interface CommitMultiStore
// used by export cmd
func (rs *Store) LoadVersion(ver int64) error {
	return rs.LoadVersionAndUpgrade(ver, nil)
}

// SetInterBlockCache is a noop here because memiavl do caching on it's own, which works well with zero-copy.
func (rs *Store) SetInterBlockCache(c types.MultiStorePersistentCache) {}

// SetInitialVersion Implements interface CommitMultiStore
// used by InitChain when the initial height is bigger than 1
func (rs *Store) SetInitialVersion(version int64) error {
	if err := rs.db.SetInitialVersion(version); err != nil {
		return err
	}

	rs.opts.InitialVersion = uint64(version)
	return nil
}

// SetIAVLCacheSize Implements interface CommitMultiStore
func (rs *Store) SetIAVLCacheSize(size int) {
}

// SetIAVLDisableFastNode Implements interface CommitMultiStore
func (rs *Store) SetIAVLDisableFastNode(disable bool) {
}

// SetIAVLSyncPruning Implements interface CommitMultiStore
func (rs *Store) SetIAVLSyncPruning(syncPruning bool) {
}

// SetLazyLoading Implements interface CommitMultiStore
func (rs *Store) SetLazyLoading(lazyLoading bool) {
}

func (rs *Store) SetMemIAVLOptions(opts memiavl.Options) {
	if opts.Logger == nil {
		opts.Logger = memiavl.Logger(rs.logger.With("module", "memiavl"))
	}
	clampHistoricalCacheOptions(&opts)
	rs.opts = opts
}

func clampHistoricalCacheOptions(opts *memiavl.Options) {
	maxInterval := int(opts.SnapshotInterval)
	if maxInterval <= 0 {
		maxInterval = memiavl.DefaultSnapshotInterval
	}

	if opts.HistoricalQueryLimit <= 0 || opts.HistoricalQueryLimit > maxInterval {
		opts.HistoricalQueryLimit = maxInterval
	}
}

// RollbackToVersion delete the versions after `target` and update the latest version.
// it should only be called in standalone cli commands.
func (rs *Store) RollbackToVersion(target int64) error {
	if target <= 0 {
		return fmt.Errorf("invalid rollback height target: %d", target)
	}

	if rs.db != nil {
		if err := rs.db.Close(); err != nil {
			return err
		}
	}

	opts := rs.opts
	opts.TargetVersion = uint64(target)
	opts.LoadForOverwriting = true

	var err error
	rs.db, err = memiavl.Load(rs.dir, opts)

	return err
}

// ListeningEnabled Implements interface CommitMultiStore
func (rs *Store) ListeningEnabled(key types.StoreKey) bool {
	if ls, ok := rs.listeners[key]; ok {
		return ls != nil
	}
	return false
}

// AddListeners Implements interface CommitMultiStore
func (rs *Store) AddListeners(keys []types.StoreKey) {
	for i := range keys {
		listener := rs.listeners[keys[i]]
		if listener == nil {
			rs.listeners[keys[i]] = types.NewMemoryListener()
		}
	}
}

// PopStateCache returns the accumulated state change messages from the CommitMultiStore
// Calling PopStateCache destroys only the currently accumulated state in each listener
// not the state in the store itself. This is a mutating and destructive operation.
// This method has been synchronized.
func (rs *Store) PopStateCache() []*types.StoreKVPair {
	var cache []*types.StoreKVPair
	for key := range rs.listeners {
		ls := rs.listeners[key]
		if ls != nil {
			cache = append(cache, ls.PopStateCache()...)
		}
	}
	sort.SliceStable(cache, func(i, j int) bool {
		return cache[i].StoreKey < cache[j].StoreKey
	})
	return cache
}

// GetStoreByName performs a lookup of a StoreKey given a store name typically
// provided in a path. The StoreKey is then used to perform a lookup and return
// a Store. If the Store is wrapped in an inter-block cache, it will be unwrapped
// prior to being returned. If the StoreKey does not exist, nil is returned.
func (rs *Store) GetStoreByName(name string) types.Store {
	key := rs.keysByName[name]
	if key == nil {
		return nil
	}

	return rs.GetCommitStore(key)
}

// Query Implements interface Queryable
func (rs *Store) Query(req *types.RequestQuery) (*types.ResponseQuery, error) {
	version := req.Height
	latestVersion := rs.LatestVersion()
	if version > latestVersion {
		return nil, sdkerrors.ErrInvalidHeight.Wrapf("requested version %d is greater than latest version %d", version, latestVersion)
	}
	if version <= 0 {
		version = rs.db.Version()
	}

	cloned, ok := rs.queryCache.GetHistoricalTrees(version)
	if !ok {
		return nil, sdkerrors.ErrInvalidHeight.Wrapf("historical version not ready: %d", version)
	}

	defer cloned.Close()

	path := req.Path
	storeName, subpath, err := parsePath(path)
	if err != nil {
		return nil, err
	}

	store := types.Queryable(memiavl.NewStore(cloned.TreeByName(storeName), rs.logger))

	// trim the path and make the query
	req.Path = subpath
	res, err := store.Query(req)
	if err != nil {
		return nil, err
	}

	if !req.Prove || !rootmulti.RequireProof(subpath) {
		return res, nil
	}

	if res.ProofOps == nil || len(res.ProofOps.Ops) == 0 {
		return nil, errors.Wrap(sdkerrors.ErrInvalidRequest, "proof is unexpectedly empty; ensure height has not been pruned")
	}

	commitInfo := convertCommitInfo(cloned.LastCommitInfo())

	// Restore origin path and append proof op.
	res.ProofOps.Ops = append(res.ProofOps.Ops, commitInfo.ProofOp(storeName))

	return res, nil
}

// parsePath expects a format like /<storeName>[/<subpath>]
// Must start with /, subpath may be empty
// Returns error if it doesn't start with /
func parsePath(path string) (storeName, subpath string, err error) {
	if !strings.HasPrefix(path, "/") {
		return storeName, subpath, errors.Wrapf(sdkerrors.ErrUnknownRequest, "invalid path: %s", path)
	}

	paths := strings.SplitN(path[1:], "/", 2)
	storeName = paths[0]

	if len(paths) == 2 {
		subpath = "/" + paths[1]
	}

	return storeName, subpath, nil
}

type storeParams struct {
	key types.StoreKey
	typ types.StoreType
}

func newStoreParams(key types.StoreKey, typ types.StoreType) storeParams {
	return storeParams{
		key: key,
		typ: typ,
	}
}

func convertCommitInfo(commitInfo *memiavl.CommitInfo) *types.CommitInfo {
	storeInfos := make([]types.StoreInfo, len(commitInfo.StoreInfos))
	for i, storeInfo := range commitInfo.StoreInfos {
		storeInfos[i] = types.StoreInfo{
			Name: storeInfo.Name,
			CommitId: types.CommitID{
				Version: storeInfo.CommitId.Version,
				Hash:    storeInfo.CommitId.Hash,
			},
		}
	}
	return &types.CommitInfo{
		Version:    commitInfo.Version,
		StoreInfos: storeInfos,
	}
}
