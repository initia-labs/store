package rootmulti

import (
	"sort"
	"sync"

	"github.com/initia-labs/store/memiavl"
	"github.com/tidwall/wal"
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
	seedWal         *wal.Log
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

// reset clears the cache without acquiring the lock.
func (c *queryDBCache) reset() {
	c.entries = make(map[int64]*cachedTrees)
	c.seq = c.seq[:0]
	if c.seedDB != nil {
		_ = c.seedDB.Close()
		c.seedDB = nil
	}
	if c.seedWal != nil {
		_ = c.seedWal.Close()
		c.seedWal = nil
	}
}

// Reset clears the cache and closes any open resources.
func (c *queryDBCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.reset()
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

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[version]; exists {
		return
	}

	trees := make(map[string]*memiavl.Tree)
	for _, entry := range db.Trees() {
		trees[entry.Name] = entry.Tree.Copy(0)
	}

	c.entries[version] = &cachedTrees{trees: trees}
	c.insertVersion(version)
	c.pruneHistoricalTrees()
}

// ReloadLatestVersion resets whole cache and adds the latest version to cache.
func (c *queryDBCache) ReloadLatestVersion(trees []memiavl.NamedTree, version int64) {
	if version <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// reset existing cache
	c.reset()

	if _, exists := c.entries[version]; exists {
		return
	}

	treeMap := make(map[string]*memiavl.Tree)
	for _, entry := range trees {
		treeMap[entry.Name] = entry.Tree.Copy(0)
	}

	c.entries[version] = &cachedTrees{trees: treeMap}
	c.insertVersion(version)
	c.pruneHistoricalTrees()
}

func (c *queryDBCache) insertVersion(version int64) {
	n := len(c.seq)
	switch {
	case n == 0:
		c.seq = append(c.seq, version)
		return
	case version > c.seq[n-1]:
		c.seq = append(c.seq, version)
		return
	case version == c.seq[n-1]:
		return
	}

	idx := sort.Search(n, func(i int) bool {
		return c.seq[i] >= version
	})
	if idx < n && c.seq[idx] == version {
		return
	}

	c.seq = append(c.seq, 0)
	copy(c.seq[idx+1:], c.seq[idx:])
	c.seq[idx] = version
}

func (c *queryDBCache) pruneHistoricalTrees() {
	limit := c.CacheLimit()
	if limit <= 0 {
		return
	}

	for len(c.seq) > limit {
		version := c.seq[0]
		c.seq = c.seq[1:]
		delete(c.entries, version)

		// clear seedDB if it's beyond the clear height
		if version >= c.seedClearHeight {
			if c.seedDB != nil {
				_ = c.seedDB.Close()
				c.seedDB = nil
			}
			if c.seedWal != nil {
				_ = c.seedWal.Close()
				c.seedWal = nil
			}
		}

	}
}

func (c *queryDBCache) CacheLimit() int {
	maxInterval := c.snapshotIntervalLimit()
	limit := c.store.opts.HistoricalQueryLimit
	if limit <= 0 || limit > maxInterval {
		limit = maxInterval
	}

	// +1 to account for the current version
	return limit + 1
}

func (c *queryDBCache) snapshotIntervalLimit() int {
	limit := int(c.store.opts.SnapshotInterval)
	if limit <= 0 {
		limit = memiavl.DefaultSnapshotInterval
	}
	return limit
}

func (c *queryDBCache) SetSeedInfo(db *memiavl.DB, wal *wal.Log, height int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seedDB = db
	c.seedWal = wal
	c.seedClearHeight = height
}
