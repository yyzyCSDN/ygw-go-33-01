package update

import (
	"errors"
	"testing"

	"zonedns/internal/journal"
	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/transfer"
	"zonedns/internal/zone"
)

func TestDdnsFailureRollsBackMemoryState(t *testing.T) {
	z := zone.New("example.com", model.ZoneMeta{
		Name:  "example.com",
		Class: "IN",
		SOA:   model.SOA{MName: "ns1.example.com", RName: "hostmaster.example.com", Serial: 100},
	}, record.New())
	store := zone.NewStore()
	store.Put(z)
	durability := journal.NewInMemory()
	durability.SetFail(func(journal.Entry) error { return errors.New("disk full") })
	svc := New(store, durability, transfer.New(store, transfer.NewMemoryNotifier()), nil)
	ch := model.Change{
		Kind:   model.ChangeUpsert,
		Record: model.Record{Name: "www.example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.99"},
	}
	if err := svc.Apply("example.com", ch); err == nil {
		t.Fatal("expected apply failure")
	}
	if _, ok := z.Lookup("www.example.com", model.TypeA); ok {
		t.Fatalf("failed update remains visible in memory")
	}
	if z.Serial() != 100 {
		t.Fatalf("serial moved after failed update: %d", z.Serial())
	}
}
