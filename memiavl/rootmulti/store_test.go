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
