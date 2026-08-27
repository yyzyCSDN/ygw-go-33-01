package transfer

import (
	"testing"

	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/zone"
)

func TestTransferSnapshotMatchesDeliveredSerial(t *testing.T) {
	z := zone.New("example.com", model.ZoneMeta{
		Name:  "example.com",
		Class: "IN",
		SOA:   model.SOA{MName: "ns1.example.com", RName: "hostmaster.example.com", Serial: 100},
	}, record.New())
	store := zone.NewStore()
	store.Put(z)
	svc := New(store, NewMemoryNotifier())
	if _, err := svc.StartTransfer("example.com"); err != nil {
		t.Fatal(err)
	}
	staged, err := z.StageChange(model.Change{
		Kind:   model.ChangeUpsert,
		Record: model.Record{Name: "api.example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.66"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Commit(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.AXFR("example.com")
	if err != nil {
		t.Fatalf("axfr: %v", err)
	}
	if snapshot.Serial != z.Serial() {
		t.Fatalf("snapshot serial %d does not match zone serial %d", snapshot.Serial, z.Serial())
	}
	found := false
	for _, r := range snapshot.Records {
		if r.Name == "api.example.com" && r.RData == "192.0.2.66" {
			found = true
		}
	}
	if !found {
		t.Fatalf("record added during transfer missing from snapshot (serial %d, zone serial %d)", snapshot.Serial, z.Serial())
	}
	delta, err := svc.IXFR("example.com", snapshot.Serial)
	if err != nil {
		t.Fatalf("ixfr after snapshot: %v", err)
	}
	if delta.Range.Start != snapshot.Serial || delta.Range.End != z.Serial() {
		t.Fatalf("ixfr range %+v does not continue from snapshot serial %d", delta.Range, snapshot.Serial)
	}
}
