package rootmulti

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

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

	mt, latest, ok := store.queryCache.GetHistoricalTrees(1)
	require.True(t, ok)
	require.Equal(t, int64(1), latest)
	require.NotNil(t, mt.TreeByName(key.Name()))
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
