package rootmulti

import (
	"fmt"
	"sort"
	"sync"

	"github.com/initia-labs/store/memiavl"
)

type queryDBCache struct {
	store *Store

	mu      sync.Mutex
	entries map[int64]*memiavl.MultiTree
	seq     []int64

	// the exporter used for (cosmos-sdk) snapshots
	exporter *snapshotExporter

	stop chan struct{}
	once sync.Once
}

func newQueryDBCache(store *Store) *queryDBCache {
	cache := &queryDBCache{
		store:   store,
		entries: make(map[int64]*memiavl.MultiTree),
		stop:    make(chan struct{}),
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

	if c.exporter != nil {
		if err := c.exporter.Close(); err != nil {
			c.store.logger.Error("failed to close snapshot MultiTree", "error", err)
		}
	}

	c.entries = make(map[int64]*memiavl.MultiTree)
	c.exporter = nil
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

// GetHistoricalTrees returns the cloned MultiTree at the given version if it exists in the cache.
// It first checks the entries map, then the snapshots map.
func (c *queryDBCache) GetHistoricalTrees(version int64) (*memiavl.MultiTree, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[version]; !ok {
		if c.exporter != nil && c.exporter.version == version {
			return c.exporter.mtree.Copy(0), true
		}
		return nil, false
	} else {
		return entry.Copy(0), true
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

// SetSnapshotExporter adds the snapshot MultiTree at the given version.
func (c *queryDBCache) SetSnapshotExporter(version int64, mtree *memiavl.MultiTree, fl memiavl.FileLock) error {
	if mtree == nil || version <= 0 {
		return fmt.Errorf("invalid snapshot MultiTree or version")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.exporter != nil {
		return fmt.Errorf("snapshot exporter already exists")
	}

	c.exporter = &snapshotExporter{
		version: version,
		mtree:   mtree,
		fl:      fl,
	}

	return nil
}

// PurgeSnapshotExporter closes and removes the snapshot MultiTree at the given version.
func (c *queryDBCache) PurgeSnapshotExporter(height int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.exporter != nil && c.exporter.version == height {
		if err := c.exporter.Close(); err != nil {
			c.store.logger.Error("failed to close snapshot MultiTree", "version", c.exporter.version, "error", err)
		}

		c.exporter = nil
	}
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
