package cache

import (
	"strings"
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

// normalizeKey lowercases the name and strips a trailing dot, mirroring
// record.CanonicalName. The cache must key every case/spelling variant of a
// name onto the same entry; otherwise an upsert and a query spelled in
// different cases would hit different cache slots and the query would keep
// returning the stale value cached before the update.
func normalizeKey(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSuffix(lower, ".")
}

func cacheKey(name string, rtype model.RecordType, nxdomain bool) string {
	kind := "pos"
	if nxdomain {
		kind = "neg"
	}
	return normalizeKey(name) + "\x00" + rtype.String() + "\x00" + kind
}

// Invalidate drops every cached entry (positive and negative, all types) for
// the given name, so a freshly applied record change is visible to all
// queries immediately instead of waiting for the old entry's TTL to elapse.
func (c *Cache) Invalidate(name string) {
	prefix := normalizeKey(name) + "\x00"
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
}

func (c *Cache) Get(name string, rtype model.RecordType, nxdomain bool) (Result, bool) {
	key := cacheKey(name, rtype, nxdomain)
	c.mu.RLock()
	e, ok := c.entries[key]
	if !ok || e.expired(c.now()) {
		c.mu.RUnlock()
		return Result{}, false
	}
	if e.nxdomain && !NegativeValid(e.storedAt, e.ttl, c.now()) {
		c.mu.RUnlock()
		return Result{}, false
	}
	result := Result{
		Records:  append([]model.Record(nil), e.data...),
		NXDomain: e.nxdomain,
		TTL:      e.ttl,
	}
	c.mu.RUnlock()
	return result, true
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
