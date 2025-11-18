package rootmulti

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/wal"

	"github.com/initia-labs/store/memiavl"
)

func TestQueryCacheHistoricalPruning(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	for i := 0; i < 3; i++ {
		commitValue(t, store, key, fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}

	historicalTrees := store.queryCache.entries
	require.Len(t, historicalTrees, store.queryCache.CacheLimit(), "historical cache should keep snapshotInterval historical versions plus the latest")
	_, hasV2 := historicalTrees[2]
	_, hasV3 := historicalTrees[3]
	require.True(t, hasV2 && hasV3, "expected versions 2 and 3 to remain")
}

func TestQueryCacheGetHistoricalTrees(t *testing.T) {
	store, key := newTestStore(t, 5)
	defer store.Close()

	commitValue(t, store, key, "foo", "bar")

	trees, ok := store.queryCache.GetHistoricalTrees(1)
	require.True(t, ok)
	require.Contains(t, trees, key.Name())
	require.NotNil(t, trees[key.Name()])
}

func TestQueryCacheReset(t *testing.T) {
	store, key := newTestStore(t, 5)
	defer store.Close()

	commitValue(t, store, key, "foo", "bar")
	require.NotEmpty(t, store.queryCache.entries)

	store.queryCache.Reset()
	require.Empty(t, store.queryCache.entries)
}

func TestQueryCacheRespectsConfiguredLimit(t *testing.T) {
	store, key := newTestStore(t, 5, func(opts *memiavl.Options) {
		opts.HistoricalQueryLimit = 2
	})
	defer store.Close()

	for i := range 4 {
		commitValue(t, store, key, fmt.Sprintf("limit-%d", i), fmt.Sprintf("v%d", i))
	}

	require.Len(t, store.queryCache.entries, store.queryCache.CacheLimit())
	_, hasOld := store.queryCache.entries[1]
	_, hasLatest := store.queryCache.entries[4]
	require.False(t, hasOld)
	require.True(t, hasLatest)
}

func TestQueryCachePrunesSeedDBWhenClearingHistoricalEntries(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	for i := range 3 {
		commitValue(t, store, key, fmt.Sprintf("prune-%d", i), fmt.Sprintf("v%d", i))
	}

	cache := store.queryCache
	opts := store.opts
	opts.ReadOnly = true
	seedDB, err := memiavl.Load(store.dir, opts)
	require.NoError(t, err)

	cache.SetSeedInfo(seedDB, &wal.Log{}, 3)
	require.NotNil(t, cache.seedDB)

	commitValue(t, store, key, "prune-2", "v2")
	require.NotNil(t, cache.seedDB, "seed db should remain until clear height is pruned")
	require.NotNil(t, cache.seedWal, "seed wal should remain until clear height is pruned")

	commitValue(t, store, key, "prune-3", "v3")
	require.NotNil(t, cache.seedDB, "seed db should remain until clear height is pruned")
	require.NotNil(t, cache.seedWal, "seed wal should remain until clear height is pruned")

	commitValue(t, store, key, "prune-4", "v4")
	require.Nil(t, cache.seedDB, "seed db should be cleared once version >= clear height is pruned")
	require.Nil(t, cache.seedWal, "seed wal should be cleared once version >= clear height is pruned")
}

func TestQueryCachePruningAfterLoadKeepsNewestVersions(t *testing.T) {
	store, key := newTestStore(t, 3)
	defer store.Close()

	for i := 0; i < 5; i++ {
		commitValue(t, store, key, fmt.Sprintf("reload-%d", i), fmt.Sprintf("v%d", i))
	}

	latest := store.LastCommitID().Version

	// Mimic node restart where the latest version is cached before background seeding.
	store.queryCache.Reset()
	store.queryCache.AddHistoricalVersion(store.db, latest)
	store.loadQueryCache(latest)

	limit := store.queryCache.CacheLimit()
	require.Len(t, store.queryCache.entries, limit)

	commitValue(t, store, key, "post-load", "latest")

	newLatest := store.LastCommitID().Version
	require.Equal(t, latest+1, newLatest)
	require.Len(t, store.queryCache.entries, limit)

	start := newLatest - int64(limit) + 1
	for v := start; v <= newLatest; v++ {
		_, ok := store.queryCache.entries[v]
		require.Truef(t, ok, "expected cached version %d", v)
	}
}
