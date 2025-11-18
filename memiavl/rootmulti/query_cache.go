package rootmulti

import (
	"sync"

	"github.com/initia-labs/store/memiavl"
)

type cachedTrees struct {
	trees map[string]*memiavl.Tree
}

type queryDBCache struct {
	store   *Store
	mu      sync.Mutex
	entries map[int64]*cachedTrees
	seq     []int64

	stop chan struct{}
	once sync.Once

	// seedDB tracks the initial memiavl.DB used to seed the query cache.
	seedDB          *memiavl.DB
	seedClearHeight int64
}

func newQueryDBCache(store *Store) *queryDBCache {
	cache := &queryDBCache{
		store:   store,
		entries: make(map[int64]*cachedTrees),
		stop:    make(chan struct{}),
	}
	return cache
}

func (c *queryDBCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[int64]*cachedTrees)
	c.seq = c.seq[:0]
	if c.seedDB != nil {
		_ = c.seedDB.Close()
		c.seedDB = nil
	}
}

func (c *queryDBCache) Close() {
	c.once.Do(func() { close(c.stop) })
	c.Reset()
}

func (c *queryDBCache) GetHistoricalTrees(version int64) (map[string]*memiavl.Tree, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[version]
	if !ok {
		return nil, false
	}
	return entry.trees, true
}

func (c *queryDBCache) AddHistoricalVersion(db *memiavl.DB, version int64) {
	if version <= 0 {
		return
	}

	trees := make(map[string]*memiavl.Tree)
	for _, entry := range db.Trees() {
		trees[entry.Name] = entry.Tree.Copy(0)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[version] = &cachedTrees{trees: trees}
	c.seq = append(c.seq, version)
	c.pruneHistoricalTrees()
}

func (c *queryDBCache) pruneHistoricalTrees() {
	limit := c.HistoricalCacheSize()
	if limit <= 0 {
		return
	}

	for len(c.seq) > limit {
		version := c.seq[0]
		c.seq = c.seq[1:]
		delete(c.entries, version)

		// clear seedDB if it's beyond the clear height
		if c.seedDB != nil && version >= c.seedClearHeight {
			_ = c.seedDB.Close()
			c.seedDB = nil
		}
	}
}

func (c *queryDBCache) HistoricalCacheSize() int {
	maxInterval := c.snapshotIntervalLimit()
	limit := c.store.opts.HistoricalQueryCacheSize
	if limit <= 0 || limit > maxInterval {
		return maxInterval
	}
	return limit
}

func (c *queryDBCache) snapshotIntervalLimit() int {
	limit := int(c.store.opts.SnapshotInterval)
	if limit <= 0 {
		limit = memiavl.DefaultSnapshotInterval
	}
	return limit
}

func (c *queryDBCache) SetSeedInfo(db *memiavl.DB, height int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seedDB = db
	c.seedClearHeight = height
}
