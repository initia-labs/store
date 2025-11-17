package rootmulti

import (
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

func TestCacheMultiStoreWithVersionUsesLiveSnapshots(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	const targetKey = "foo"

	commitValue(t, store, key, targetKey, "v1")
	commitValue(t, store, key, targetKey, "v2")

	require.Len(t, store.queryCache.entries, 0)

	cms, err := store.CacheMultiStoreWithVersion(1)
	require.NoError(t, err)
	require.Len(t, store.queryCache.entries, 0, "live snapshot should serve queries without loading historical DB")

	value := cms.GetKVStore(key).Get([]byte(targetKey))
	require.Equal(t, []byte("v1"), value)
}

func TestCacheMultiStoreWithVersionLoadsHistoricalDB(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	const targetKey = "bar"

	commitValue(t, store, key, targetKey, "v1")
	commitValue(t, store, key, targetKey, "v2")
	commitValue(t, store, key, targetKey, "v3")
	commitValue(t, store, key, targetKey, "v4")

	require.Len(t, store.queryCache.entries, 0)

	cms, err := store.CacheMultiStoreWithVersion(1)
	require.NoError(t, err)
	require.Len(t, store.queryCache.entries, 1, "historical queries should load read-only DB once pruned from live cache")

	value := cms.GetKVStore(key).Get([]byte(targetKey))
	require.Equal(t, []byte("v1"), value)
}

func newTestStore(t *testing.T, snapshotInterval uint32) (*Store, types.StoreKey) {
	t.Helper()

	store := NewStore(t.TempDir(), log.NewNopLogger(), false)
	store.SetMemIAVLOptions(memiavl.Options{
		SnapshotInterval: snapshotInterval,
	})

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
