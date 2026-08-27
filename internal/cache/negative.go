package cache

import (
	"time"

	"zonedns/internal/model"
)

func NegativeTTL(soa model.SOA) uint32 {
	return soa.Minimum
}

func (c *Cache) StoreNegative(query model.Query, soa model.SOA) {
	ttl := NegativeTTL(soa)
	c.Put(query.Name, query.Type, true, nil, ttl)
}

func ExpireTime(now time.Time, ttl uint32) time.Time {
	return now.Add(time.Duration(ttl) * time.Second)
}

func NegativeValid(storedAt time.Time, ttl uint32, now time.Time) bool {
	return now.Sub(storedAt) < time.Duration(ttl)*time.Second
}
