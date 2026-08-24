package record

import (
	"testing"

	"zonedns/internal/model"
)

func TestCaseFoldConsistentAcrossWriteAndQuery(t *testing.T) {
	s := New()
	rec := model.Record{Name: "WWW.Example.COM", Type: model.TypeA, TTL: 300, RData: "192.0.2.10"}
	if _, err := s.Apply(model.Change{Kind: model.ChangeUpsert, Record: rec}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"www.example.com", "WWW.EXAMPLE.COM", "WwW.ExAmPlE.CoM"} {
		records, ok := s.Lookup(name, model.TypeA)
		if !ok || len(records) == 0 {
			t.Fatalf("lookup %q missed the stored record", name)
		}
		if records[0].RData != "192.0.2.10" {
			t.Fatalf("lookup %q returned %q", name, records[0].RData)
		}
	}
	if all := s.All(); len(all) != 1 {
		t.Fatalf("expected one stored version, got %d", len(all))
	}
}
