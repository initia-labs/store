package rootmulti

import (
	"sort"
	"sync"

	"github.com/initia-labs/store/memiavl"
)

type queryDBCache struct {
	store *Store

	mu      sync.Mutex
	entries map[int64]*memiavl.MultiTree
	seq     []int64

	// the trees used for (cosmos-sdk) snapshots
	snapshots map[int64]*memiavl.MultiTree

	stop chan struct{}
	once sync.Once
}

func newQueryDBCache(store *Store) *queryDBCache {
	cache := &queryDBCache{
		store:   store,
		entries: make(map[int64]*memiavl.MultiTree),
		stop:    make(chan struct{}),

		snapshots: make(map[int64]*memiavl.MultiTree),
	}
	return cache
}

// reset clears the cache without acquiring the lock.
func (c *queryDBCache) reset() {

	// close all MultiTrees
	for _, mt := range c.entries {
		if err := mt.Close(); err != nil {
			c.store.logger.Error("failed to close historical MultiTree", "error", err)
		}
	}

	for _, mt := range c.snapshots {
		if err := mt.Close(); err != nil {
			c.store.logger.Error("failed to close snapshot MultiTree", "error", err)
		}
	}

	c.entries = make(map[int64]*memiavl.MultiTree)
	c.snapshots = make(map[int64]*memiavl.MultiTree)
	c.seq = c.seq[:0]
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

// GetHistoricalTrees returns the MultiTree at the given version if it exists in the cache.
// It first checks the entries map, then the snapshots map.
func (c *queryDBCache) GetHistoricalTrees(version int64) (*memiavl.MultiTree, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[version]; !ok {
		if mtree, ok := c.snapshots[version]; ok {
			return mtree, true
		}
		return nil, false
	} else {
		return entry, true
	}
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

	c.entries[version] = db.MultiTree.Copy(0)
	c.insertVersion(version)
	c.pruneHistoricalTrees()
}

// AddSnapshotVersion adds the snapshot MultiTree at the given version.
func (c *queryDBCache) AddSnapshotVersion(mtree *memiavl.MultiTree, version int64) {
	if mtree == nil || version <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.snapshots[version]; exists {
		return
	}

	c.snapshots[version] = mtree
}

// RemoveSnapshotVersion closes and removes the snapshot MultiTree at the given version.
func (c *queryDBCache) RemoveSnapshotVersion(version int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if mtree, exists := c.snapshots[version]; exists {
		if mtree != nil {
			_ = mtree.Close()
		}
	}

	delete(c.snapshots, version)
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

		mt := c.entries[version]
		if err := mt.Close(); err != nil {
			c.store.logger.Error("failed to close historical MultiTree", "version", version, "error", err)
		}

		delete(c.entries, version)
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
