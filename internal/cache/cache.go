package cache

import (
	"sync"
	"time"

	"zonedns/internal/model"
)

type Result struct {
	Records  []model.Record
	NXDomain bool
	TTL      uint32
}

type entry struct {
	key      string
	data     []model.Record
	nxdomain bool
	ttl      uint32
	storedAt time.Time
}

type Cache struct {
	mu         sync.RWMutex
	entries    map[string]*entry
	maxEntries int
	now        func() time.Time
}

func New(maxEntries int) *Cache {
	return &Cache{
		entries:    make(map[string]*entry),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func cacheKey(name string, rtype model.RecordType, nxdomain bool) string {
	kind := "pos"
	if nxdomain {
		kind = "neg"
	}
	return name + "\x00" + rtype.String() + "\x00" + kind
}

func (c *Cache) Get(name string, rtype model.RecordType, nxdomain bool) (Result, bool) {
	key := cacheKey(name, rtype, nxdomain)
	e := c.entries[key]
	if e == nil {
		return Result{}, false
	}
	return Result{Records: e.data, NXDomain: e.nxdomain, TTL: e.ttl}, true
}

func (c *Cache) Put(name string, rtype model.RecordType, nxdomain bool, records []model.Record, ttl uint32) {
	key := cacheKey(name, rtype, nxdomain)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxEntries && c.entries[key] == nil {
		c.evictLocked()
	}
	c.entries[key] = &entry{
		key:      key,
		data:     append([]model.Record(nil), records...),
		nxdomain: nxdomain,
		ttl:      ttl,
		storedAt: c.now(),
	}
}

func (c *Cache) SweepExpired(now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for key, e := range c.entries {
		if e.expired(now) {
			e.data = nil
			delete(c.entries, key)
			removed++
		}
	}
	return removed
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (e *entry) expired(now time.Time) bool {
	return !now.Before(ExpireTime(e.storedAt, e.ttl))
}

func (c *Cache) evictLocked() {

	var oldestKey string
	var oldest time.Time
	for key, e := range c.entries {
		if oldestKey == "" || e.storedAt.Before(oldest) {
			oldestKey = key
			oldest = e.storedAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
