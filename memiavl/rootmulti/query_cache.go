package rootmulti

import (
	"sync"
	"time"

	"github.com/initia-labs/store/memiavl"
)

const (
	queryCacheMaxEntries = 8
	queryCacheTTL        = 2 * time.Minute
)

type cachedQueryDB struct {
	db       *memiavl.DB
	lastUsed time.Time
}

type cachedLiveTrees struct {
	trees map[string]*memiavl.Tree
}

type queryDBCache struct {
	store   *Store
	mu      sync.Mutex
	entries map[int64]*cachedQueryDB
	live    map[int64]*cachedLiveTrees
	liveSeq []int64

	stop chan struct{}
	once sync.Once
}

func newQueryDBCache(store *Store) *queryDBCache {
	cache := &queryDBCache{
		store:   store,
		entries: make(map[int64]*cachedQueryDB),
		live:    make(map[int64]*cachedLiveTrees),
		stop:    make(chan struct{}),
	}
	cache.startEvictor()
	return cache
}

func (c *queryDBCache) GetOrLoad(version int64) (*memiavl.DB, error) {
	now := time.Now()

	c.mu.Lock()
	if entry, ok := c.entries[version]; ok {
		entry.lastUsed = now
		c.pruneLocked(now)
		c.mu.Unlock()
		return entry.db, nil
	}
	c.mu.Unlock()

	db, err := c.load(version)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if entry, ok := c.entries[version]; ok {
		entry.lastUsed = now
		c.pruneLocked(now)
		c.mu.Unlock()
		_ = db.Close()
		return entry.db, nil
	}
	c.entries[version] = &cachedQueryDB{db: db, lastUsed: now}
	c.pruneLocked(now)
	c.mu.Unlock()

	return db, nil
}

func (c *queryDBCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for version, entry := range c.entries {
		_ = entry.db.Close()
		delete(c.entries, version)
	}
	c.live = make(map[int64]*cachedLiveTrees)
	c.liveSeq = c.liveSeq[:0]
}

func (c *queryDBCache) Close() {
	c.once.Do(func() { close(c.stop) })
	c.Reset()
}

func (c *queryDBCache) GetLiveTrees(version int64) (map[string]*memiavl.Tree, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.live[version]
	if !ok {
		return nil, false
	}
	return entry.trees, true
}

func (c *queryDBCache) load(version int64) (*memiavl.DB, error) {
	opts := c.store.opts
	opts.TargetVersion = uint32(version)
	opts.ReadOnly = true
	opts.CreateIfMissing = false
	return memiavl.Load(c.store.dir, opts)
}

func (c *queryDBCache) AddLiveVersion(db *memiavl.DB, version int64) {
	if version <= 0 {
		return
	}

	trees := make(map[string]*memiavl.Tree)
	for _, entry := range db.Trees() {
		trees[entry.Name] = entry.Tree.Copy(c.store.opts.CacheSize)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.live[version] = &cachedLiveTrees{trees: trees}
	c.liveSeq = append(c.liveSeq, version)
	c.pruneLiveLocked()
}

func (c *queryDBCache) pruneLocked(now time.Time) {
	for version, entry := range c.entries {
		if now.Sub(entry.lastUsed) > queryCacheTTL {
			_ = entry.db.Close()
			delete(c.entries, version)
		}
	}

	for len(c.entries) > queryCacheMaxEntries {
		var oldestVersion int64
		var oldestTime time.Time
		first := true
		for version, entry := range c.entries {
			if first || entry.lastUsed.Before(oldestTime) {
				first = false
				oldestVersion = version
				oldestTime = entry.lastUsed
			}
		}
		if entry, ok := c.entries[oldestVersion]; ok {
			_ = entry.db.Close()
			delete(c.entries, oldestVersion)
		}
	}
}

func (c *queryDBCache) pruneLiveLocked() {
	limit := c.liveKeepLimit()
	if limit <= 0 {
		return
	}
	for len(c.liveSeq) > limit {
		version := c.liveSeq[0]
		c.liveSeq = c.liveSeq[1:]
		delete(c.live, version)
	}
}

func (c *queryDBCache) startEvictor() {
	go func() {
		ticker := time.NewTicker(queryCacheTTL / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.mu.Lock()
				c.pruneLocked(time.Now())
				c.mu.Unlock()
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *queryDBCache) liveKeepLimit() int {
	limit := int(c.store.opts.SnapshotInterval)
	if limit <= 0 {
		limit = memiavl.DefaultSnapshotInterval
	}
	return limit
}
