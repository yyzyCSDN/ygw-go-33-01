package resolver_test

import (
	"testing"

	"zonedns/internal/cache"
	"zonedns/internal/journal"
	"zonedns/internal/metric"
	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/resolver"
	"zonedns/internal/transfer"
	"zonedns/internal/update"
	"zonedns/internal/zone"
)

// newZone builds an authoritative zone with a single A record seed.
func newZone(name string) *zone.Zone {
	meta := model.ZoneMeta{
		Name:  name,
		Class: "IN",
		SOA: model.SOA{
			MName: "ns1." + name, RName: "hostmaster." + name,
			Serial: 2026082201, Refresh: 3600, Retry: 600, Expire: 86400, Minimum: 300,
		},
		Primary: true,
	}
	recs := record.New()
	z := zone.New(name, meta, recs)
	if _, err := recs.Apply(model.Change{
		Kind: model.ChangeUpsert,
		Record: model.Record{Name: "www." + name, Type: model.TypeA, TTL: 300, RData: "192.0.2.10"},
	}); err != nil {
		panic(err)
	}
	return z
}

// TestUpdateVisibleAcrossCase reproduces the reported defect: after a record
// is updated (under a mixed-case name), every case spelling of the query —
// both lowercase and mixed-case — must return the same updated value, with no
// stale cached answer surviving under any casing.
//
// Before the fix the record store keyed writes by raw case while lookups used
// the canonical name, and the response cache keyed by raw case with no
// invalidation on update — so the two spellings returned different values.
func TestUpdateVisibleAcrossCase(t *testing.T) {
	zones := zone.NewStore()
	const zoneName = "example.com"
	zones.Put(newZone(zoneName))

	responseCache := cache.New(64)
	metrics := metric.New()
	transfers := transfer.New(zones, transfer.NewMemoryNotifier())
	updates := update.New(zones, journal.NewInMemory(), transfers, metrics, responseCache)
	// signer is nil: the resolver skips DNSSEC verification, so no keyring is
	// required to exercise the query/update path.
	resolverService := resolver.New(zones, responseCache, nil, metrics)

	// 1. Populate the cache with the seeded (old) value via a lowercase query.
	old, err := resolverService.Resolve(model.Query{Name: "www.example.com", Type: model.TypeA})
	if err != nil {
		t.Fatalf("initial resolve: %v", err)
	}
	if old.RData != "192.0.2.10" {
		t.Fatalf("initial resolve = %q, want seeded 192.0.2.10", old.RData)
	}

	// 2. Update the record under a mixed-case name.
	if err := updates.Apply(zoneName, model.Change{
		Kind: model.ChangeUpsert,
		Record: model.Record{
			Name: "WWW.Example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.55",
		},
	}); err != nil {
		t.Fatalf("apply update: %v", err)
	}

	// 3. Both casings must now return the new value — no stale cache, no fork.
	for _, query := range []string{"www.example.com", "WWW.EXAMPLE.COM", "Www.Example.com"} {
		got, err := resolverService.Resolve(model.Query{Name: query, Type: model.TypeA})
		if err != nil {
			t.Fatalf("Resolve(%q) after update: %v", query, err)
		}
		t.Logf("Resolve(%q) = rdata=%q nx=%v", query, got.RData, got.NXDomain)
		if got.RData != "192.0.2.55" {
			t.Fatalf("Resolve(%q).RData = %q, want updated 192.0.2.55 (stale value returned)", query, got.RData)
		}
	}

	// 4. Exactly one authoritative record must exist — no case-forked copy.
	z, err := zones.Get(zoneName)
	if err != nil {
		t.Fatalf("get zone: %v", err)
	}
	records, ok := z.Lookup("www.example.com", model.TypeA)
	if !ok || len(records) != 1 {
		t.Fatalf("zone holds %d A records for www.example.com, want exactly one (no case-fork)", len(records))
	}
}

// TestUpdateDeleteVisibleAcrossCase verifies that a delete is likewise visible
// to all casings: a name that previously resolved must go NX/empty after a
// mixed-case delete, instead of returning a stale cached answer.
func TestUpdateDeleteVisibleAcrossCase(t *testing.T) {
	zones := zone.NewStore()
	const zoneName = "example.com"
	zones.Put(newZone(zoneName))

	responseCache := cache.New(64)
	metrics := metric.New()
	transfers := transfer.New(zones, transfer.NewMemoryNotifier())
	updates := update.New(zones, journal.NewInMemory(), transfers, metrics, responseCache)
	resolverService := resolver.New(zones, responseCache, nil, metrics)

	// Cache the positive answer first.
	if _, err := resolverService.Resolve(model.Query{Name: "www.example.com", Type: model.TypeA}); err != nil {
		t.Fatalf("initial resolve: %v", err)
	}

	// Delete under a different casing.
	if err := updates.Apply(zoneName, model.Change{
		Kind: model.ChangeDelete,
		Record: model.Record{
			Name: "WWW.example.com", Type: model.TypeA, RData: "192.0.2.10",
		},
	}); err != nil {
		t.Fatalf("apply delete: %v", err)
	}

	// Every casing must reflect the deletion rather than a stale positive hit.
	for _, query := range []string{"www.example.com", "WWW.EXAMPLE.COM"} {
		got, err := resolverService.Resolve(model.Query{Name: query, Type: model.TypeA})
		if err != nil {
			t.Fatalf("Resolve(%q) after delete: %v", query, err)
		}
		if !got.NXDomain {
			t.Fatalf("Resolve(%q) after delete = %+v, want NXDomain (stale positive answer returned)", query, got)
		}
	}
}
