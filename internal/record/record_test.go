package record

import (
	"testing"

	"zonedns/internal/model"
)

// TestApplyCanonicalizesName reproduces the case-fork bug: a record upserted
// under a mixed-case name must be stored under its canonical (lowercased,
// trailing-dot-stripped) key, so that a subsequent canonical lookup — of any
// case spelling — returns the very same record rather than forking the name
// into two separate stored copies.
func TestApplyCanonicalizesName(t *testing.T) {
	store := New()

	if _, err := store.Apply(model.Change{
		Kind:   model.ChangeUpsert,
		Record: model.Record{Name: "WWW.example.com.", Type: model.TypeA, TTL: 60, RData: "192.0.2.10"},
	}); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	// Every case variant must resolve to the single canonical record.
	for _, query := range []string{"www.example.com", "WWW.EXAMPLE.COM", "Www.Example.com", "www.example.com."} {
		records, ok := store.Lookup(query, model.TypeA)
		if !ok || len(records) != 1 {
			t.Fatalf("Lookup(%q) = ok=%v records=%d, want exactly one", query, ok, len(records))
		}
		if records[0].RData != "192.0.2.10" {
			t.Fatalf("Lookup(%q).RData = %q, want 192.0.2.10", query, records[0].RData)
		}
		if records[0].Name != "www.example.com" {
			t.Fatalf("Lookup(%q).Name = %q, want canonical www.example.com", query, records[0].Name)
		}
	}

	if got := len(store.Names()); got != 1 {
		t.Fatalf("store holds %d distinct names, want 1 (no case-fork)", got)
	}
}

// TestApplyUpdateReplacesAcrossCase asserts that re-upserting a record under
// a different case spelling merges onto the same canonical entry rather than
// forking a second stored copy keyed by case. An upsert of identical rdata
// across casings must dedup to a single record.
func TestApplyUpdateReplacesAcrossCase(t *testing.T) {
	store := New()
	upsert := func(name, rdata string) {
		t.Helper()
		if _, err := store.Apply(model.Change{
			Kind:   model.ChangeUpsert,
			Record: model.Record{Name: name, Type: model.TypeA, TTL: 60, RData: rdata},
		}); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}

	upsert("www.example.com", "192.0.2.10") // lowercase
	upsert("WWW.example.com", "192.0.2.10") // same rdata, different case — must dedup, not fork

	records, ok := store.Lookup("Www.Example.com", model.TypeA)
	if !ok || len(records) != 1 {
		t.Fatalf("Lookup after re-upsert = ok=%v records=%d, want exactly one deduped record (no case-fork)", ok, len(records))
	}
	if got := len(store.Names()); got != 1 {
		t.Fatalf("store holds %d distinct names after cross-case upsert, want 1 (no case-fork)", got)
	}
}

// TestApplyDeleteAcrossCase verifies a delete issued under one case spelling
// removes a record that was upserted under a different spelling.
func TestApplyDeleteAcrossCase(t *testing.T) {
	store := New()
	if _, err := store.Apply(model.Change{
		Kind:   model.ChangeUpsert,
		Record: model.Record{Name: "WWW.example.com", Type: model.TypeA, TTL: 60, RData: "192.0.2.10"},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := store.Apply(model.Change{
		Kind:   model.ChangeDelete,
		Record: model.Record{Name: "www.example.com", Type: model.TypeA, RData: "192.0.2.10"},
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if records, ok := store.Lookup("WWW.EXAMPLE.com", model.TypeA); ok || len(records) != 0 {
		t.Fatalf("Lookup after delete = ok=%v records=%d, want gone", ok, len(records))
	}
}
