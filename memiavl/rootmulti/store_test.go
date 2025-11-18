package rootmulti

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	"cosmossdk.io/store/types"

	"github.com/initia-labs/store/memiavl"
)

func TestLastCommitID(t *testing.T) {
	store := NewStore(t.TempDir(), log.NewNopLogger(), false)
	require.Equal(t, types.CommitID{}, store.LastCommitID())
}

func TestCacheMultiStoreWithVersionServesCachedVersions(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	const targetKey = "foo"

	commitValue(t, store, key, targetKey, "v1")
	commitValue(t, store, key, targetKey, "v2")

	require.Len(t, store.queryCache.entries, 2)

	cms, err := store.CacheMultiStoreWithVersion(1)
	require.NoError(t, err)

	value := cms.GetKVStore(key).Get([]byte(targetKey))
	require.Equal(t, []byte("v1"), value)
}

func TestCacheMultiStoreWithVersionReturnsErrorWhenPruned(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	const targetKey = "bar"

	commitValue(t, store, key, targetKey, "v1")
	commitValue(t, store, key, targetKey, "v2")
	commitValue(t, store, key, targetKey, "v3")
	commitValue(t, store, key, targetKey, "v4")

	require.Len(t, store.queryCache.entries, 2)

	_, err := store.CacheMultiStoreWithVersion(1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "historical version not found")
}

func TestLoadQueryCachePopulatesHistoricalWindow(t *testing.T) {
	store, key := newTestStore(t, 3)
	defer store.Close()

	for i := range 5 {
		commitValue(t, store, key, "foo", fmt.Sprintf("v%d", i))
	}

	latest := store.LastCommitID().Version
	require.True(t, latest > int64(store.opts.HistoricalQueryLimit))

	// mimic restart: keep only the latest version in cache before loading from disk
	store.queryCache.Reset()
	store.queryCache.AddHistoricalVersion(store.db, latest)
	require.Len(t, store.queryCache.entries, 1)

	store.loadQueryCache(latest)

	limit := store.opts.HistoricalQueryLimit
	require.Len(t, store.queryCache.entries, limit)

	start := latest - int64(limit) + 1
	for v := start; v <= latest; v++ {
		_, ok := store.queryCache.entries[v]
		require.Truef(t, ok, "expected cached version %d", v)
	}

	for h := start; h <= latest; h++ {
		cms, err := store.CacheMultiStoreWithVersion(int64(h))
		require.NoError(t, err)
		value := cms.GetKVStore(key).Get([]byte("foo"))
		require.Equal(t, []byte(fmt.Sprintf("v%d", h-1)), value)
	}

	require.NotNil(t, store.queryCache.seedDB)
	require.Equal(t, latest, store.queryCache.seedClearHeight)
}

func newTestStore(t *testing.T, snapshotInterval uint32, customizers ...func(*memiavl.Options)) (*Store, types.StoreKey) {
	t.Helper()

	store := NewStore(t.TempDir(), log.NewNopLogger(), false)
	opts := memiavl.Options{
		SnapshotInterval: snapshotInterval,
	}
	for _, customize := range customizers {
		customize(&opts)
	}
	store.SetMemIAVLOptions(opts)

	key := types.NewKVStoreKey("test")
	store.MountStoreWithDB(key, types.StoreTypeIAVL, nil)
	require.NoError(t, store.LoadLatestVersion())

	return store, key
}

func commitValue(t *testing.T, store *Store, key types.StoreKey, stateKey, value string) {
	t.Helper()

	kv := store.GetCommitKVStore(key)
	kv.Set([]byte(stateKey), []byte(value))
	store.Commit()
}
