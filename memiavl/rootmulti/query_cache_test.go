package rootmulti

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryCacheLivePruning(t *testing.T) {
	store, key := newTestStore(t, 2)
	defer store.Close()

	for i := 0; i < 3; i++ {
		commitValue(t, store, key, fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}

	live := store.queryCache.live
	require.Equal(t, 2, len(live), "live cache should keep only snapshotInterval versions")
	_, hasV2 := live[2]
	_, hasV3 := live[3]
	require.True(t, hasV2 && hasV3, "expected versions 2 and 3 to remain")
}

func TestQueryCacheGetLiveTrees(t *testing.T) {
	store, key := newTestStore(t, 5)
	defer store.Close()

	commitValue(t, store, key, "foo", "bar")

	trees, ok := store.queryCache.GetLiveTrees(1)
	require.True(t, ok)
	require.Contains(t, trees, key.Name())
	require.NotNil(t, trees[key.Name()])
}

func TestQueryCacheReset(t *testing.T) {
	store, key := newTestStore(t, 5)
	defer store.Close()

	commitValue(t, store, key, "foo", "bar")
	require.NotEmpty(t, store.queryCache.live)

	store.queryCache.Reset()
	require.Empty(t, store.queryCache.live)
	require.Empty(t, store.queryCache.entries)
}
