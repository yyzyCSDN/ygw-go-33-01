package zone

import (
	"testing"

	"zonedns/internal/model"
	"zonedns/internal/record"
)

func TestRecordUpdateBumpsSoaSerial(t *testing.T) {
	store := record.New()
	z := New("example.com", model.ZoneMeta{
		Name:  "example.com",
		Class: "IN",
		SOA: model.SOA{
			MName:  "ns1.example.com",
			RName:  "hostmaster.example.com",
			Serial: 2026082201,
		},
	}, store)
	before := z.Serial()
	staged, err := z.StageChange(model.Change{
		Kind:   model.ChangeUpsert,
		Record: model.Record{Name: "WWW.Example.COM", Type: model.TypeA, TTL: 300, RData: "192.0.2.10"},
	})
	if err != nil {
		t.Fatalf("stage change: %v", err)
	}
	if err := staged.Commit(); err != nil {
		t.Fatalf("commit change: %v", err)
	}
	after := z.Serial()
	if after <= before {
		t.Fatalf("SOA serial did not advance: before=%d after=%d", before, after)
	}
	if z.Meta().SOA.Serial != after {
		t.Fatalf("meta SOA serial %d does not match zone serial %d", z.Meta().SOA.Serial, after)
	}
	records, ok := z.Lookup("www.example.com", model.TypeA)
	if !ok || len(records) == 0 || records[0].RData != "192.0.2.10" {
		t.Fatalf("record not visible after committed update")
	}
}
