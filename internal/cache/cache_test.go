package cache

import (
	"testing"

	"zonedns/internal/model"
)

// TestCacheKeyCaseInsensitive asserts all case/spelling variants of a name
// share one cache slot, so a value cached under one casing is visible to a
// lookup spelled in any other casing.
func TestCacheKeyCaseInsensitive(t *testing.T) {
	c := New(8)
	c.Put("WWW.example.com", model.TypeA, false,
		[]model.Record{{Name: "WWW.example.com", Type: model.TypeA, RData: "192.0.2.10"}}, 300)

	for _, query := range []string{"www.example.com", "WWW.EXAMPLE.COM", "Www.Example.com", "www.example.com."} {
		result, ok := c.Get(query, model.TypeA, false)
		if !ok {
			t.Fatalf("Get(%q) miss, want hit (cache key must be case-insensitive)", query)
		}
		if len(result.Records) != 1 || result.Records[0].RData != "192.0.2.10" {
			t.Fatalf("Get(%q) = %+v, want the single cached record", query, result)
		}
	}
}

// TestCacheInvalidateDropsAllVariants asserts that Invalidate clears the
// cached entry so that all case variants must re-resolve, preventing a stale
// answer from masking a freshly applied update.
func TestCacheInvalidateDropsAllVariants(t *testing.T) {
	c := New(8)
	c.Put("www.example.com", model.TypeA, false,
		[]model.Record{{Name: "www.example.com", Type: model.TypeA, RData: "192.0.2.10"}}, 300)

	// Sanity: entry present before invalidation.
	if _, ok := c.Get("WWW.example.com", model.TypeA, false); !ok {
		t.Fatalf("expected cache hit before Invalidate")
	}

	c.Invalidate("WWW.Example.com")

	if _, ok := c.Get("www.example.com", model.TypeA, false); ok {
		t.Fatalf("Get after Invalidate hit; stale value could mask an update")
	}
	if _, ok := c.Get("WWW.EXAMPLE.COM", model.TypeA, false); ok {
		t.Fatalf("case-variant Get after Invalidate hit; Invalidate must clear all casings")
	}
}

// TestCacheInvalidateNegativeEntry asserts that a cached negative (NXDOMAIN)
// answer is also cleared, so a name that becomes resolvable after an update
// is no longer masked by a stale NX cache.
func TestCacheInvalidateNegativeEntry(t *testing.T) {
	c := New(8)
	c.Put("www.example.com", model.TypeA, true, nil, 300)

	c.Invalidate("WWW.example.com")

	if _, ok := c.Get("www.example.com", model.TypeA, true); ok {
		t.Fatalf("negative cache entry survived Invalidate; a later update would be masked")
	}
}

// TestCacheInvalidateScoped asserts that Invalidate only drops entries for
// the named record and leaves siblings untouched.
func TestCacheInvalidateScoped(t *testing.T) {
	c := New(8)
	c.Put("www.example.com", model.TypeA, false,
		[]model.Record{{Name: "www.example.com", Type: model.TypeA, RData: "192.0.2.10"}}, 300)
	c.Put("mail.example.com", model.TypeA, false,
		[]model.Record{{Name: "mail.example.com", Type: model.TypeA, RData: "192.0.2.20"}}, 300)

	c.Invalidate("WWW.example.com")

	if _, ok := c.Get("mail.example.com", model.TypeA, false); !ok {
		t.Fatalf("Invalidate dropped an unrelated record")
	}
	if _, ok := c.Get("www.example.com", model.TypeA, false); ok {
		t.Fatalf("Invalidate failed to drop the targeted record")
	}
}
