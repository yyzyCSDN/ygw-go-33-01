package transfer

import (
	"testing"

	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/zone"
)

func TestIxfrDeleteDeltaAppliedCorrectly(t *testing.T) {
	meta := model.ZoneMeta{
		Name:  "example.com",
		Class: "IN",
		SOA:   model.SOA{MName: "ns1.example.com", RName: "hostmaster.example.com", Serial: 100},
	}
	primary := zone.New("example.com", meta, record.New())
	secondary := zone.New("example.com", meta, record.New())
	seed := model.Record{Name: "www.example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.10"}
	if _, err := primary.Records().Apply(model.Change{Kind: model.ChangeUpsert, Record: seed}); err != nil {
		t.Fatal(err)
	}
	if _, err := secondary.Records().Apply(model.Change{Kind: model.ChangeUpsert, Record: seed}); err != nil {
		t.Fatal(err)
	}
	staged, err := primary.StageChange(model.Change{Kind: model.ChangeDelete, Record: seed})
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Commit(); err != nil {
		t.Fatal(err)
	}
	store := zone.NewStore()
	store.Put(primary)
	svc := New(store, NewMemoryNotifier())
	delta, err := svc.IXFR("example.com", 100)
	if err != nil {
		t.Fatalf("ixfr: %v", err)
	}
	if delta.Range.Start != 100 || delta.Range.End != 101 {
		t.Fatalf("delta serial range mismatch: %+v", delta.Range)
	}
	if len(delta.Ops) != 1 || delta.Ops[0].Kind != model.ChangeDelete || delta.Ops[0].Record.Name != "www.example.com" {
		t.Fatalf("delete was not delivered as a delete: %+v", delta.Ops)
	}
	if err := secondary.ApplyDelta(delta); err != nil {
		t.Fatalf("apply delta: %v", err)
	}
	if _, ok := secondary.Lookup("www.example.com", model.TypeA); ok {
		t.Fatalf("deleted record reappeared on secondary")
	}
}
