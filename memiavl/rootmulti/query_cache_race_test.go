package rootmulti

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"cosmossdk.io/store/types"
	"github.com/stretchr/testify/require"

	"github.com/initia-labs/store/memiavl"
)

// TestQueryAtLatestNeverNotReady hammers CacheMultiStoreWithVersion concurrently
// with Commit, at both the advertised latest version and version 0 (the
// "client did not specify a height" path). The advertised latest version must
// always be servable, no query may ever fail with "historical version not ready".
func TestQueryAtLatestNeverNotReady(t *testing.T) {
	store, key := newTestStore(t, 10000, func(opts *memiavl.Options) {
		opts.HistoricalQueryLimit = 30
	})
	defer store.Close()

	// seed one version so LatestVersion() is never 0
	commitValue(t, store, key, "k", "v0")

	const commits = 3000
	const queriers = 4

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var notReady, other, okCnt atomic.Int64

	classify := func(cms types.CacheMultiStore, err error) {
		switch {
		case err == nil:
			okCnt.Add(1)
			if closer, ok := cms.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		case strings.Contains(err.Error(), "historical version not ready"):
			notReady.Add(1)
		default:
			other.Add(1)
		}
	}

	for range queriers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}

				cms, err := store.CacheMultiStoreWithVersion(store.LatestVersion())
				classify(cms, err)

				cms, err = store.CacheMultiStoreWithVersion(0)
				classify(cms, err)
			}
		}()
	}

	for i := 1; i <= commits; i++ {
		commitValue(t, store, key, "k", fmt.Sprintf("v%d", i))
	}
	close(stop)
	wg.Wait()

	t.Logf("ok=%d notReady=%d other=%d", okCnt.Load(), notReady.Load(), other.Load())
	require.Zero(t, notReady.Load(), "advertised latest version must always be queryable")
	require.Zero(t, other.Load(), "unexpected error class")
	require.Positive(t, okCnt.Load())
}

// TestStartupLatestAlwaysServed mimics the state right after the synchronous
// part of LoadVersionAndUpgrade on restart. Only the latest version is seeded
// and the background WAL backfill has made no progress yet. Queries at latest
// and at height 0 must succeed immediately; older heights become available
// once the backfill completes.
func TestStartupLatestAlwaysServed(t *testing.T) {
	store, key := newTestStore(t, 10000, func(opts *memiavl.Options) {
		opts.HistoricalQueryLimit = 30
	})
	defer store.Close()

	for i := range 10 {
		commitValue(t, store, key, "k", fmt.Sprintf("v%d", i))
	}
	latest := store.LastCommitID().Version

	store.queryCache.Reset()
	store.queryCache.AddHistoricalVersion(store.db, latest)

	for _, v := range []int64{latest, 0} {
		cms, err := store.CacheMultiStoreWithVersion(v)
		require.NoError(t, err)
		closeCacheMultiStore(t, cms)
	}

	// older heights are pending until backfill reaches them
	_, err := store.CacheMultiStoreWithVersion(latest - 1)
	require.ErrorContains(t, err, "historical version not ready")

	store.loadQueryCache(latest)
	cms, err := store.CacheMultiStoreWithVersion(latest - 1)
	require.NoError(t, err)
	closeCacheMultiStore(t, cms)
}
