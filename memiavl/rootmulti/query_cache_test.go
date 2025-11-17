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
	require.Equal(t, 2, len(historicalTrees), "historical cache should keep only snapshotInterval versions")
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
		opts.HistoricalQueryCacheSize = 2
	})
	defer store.Close()

	for i := 0; i < 4; i++ {
		commitValue(t, store, key, fmt.Sprintf("limit-%d", i), fmt.Sprintf("v%d", i))
	}

	require.Len(t, store.queryCache.entries, 2)
	_, hasOld := store.queryCache.entries[2]
	_, hasLatest := store.queryCache.entries[4]
	require.False(t, hasOld)
	require.True(t, hasLatest)
}
