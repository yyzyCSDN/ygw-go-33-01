package transfer

import (
	"testing"

	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/zone"
)

// newPrimaryWithRecord builds a primary zone seeded with one A record and
// returns the zone plus the serial it had right after seeding.
func newPrimaryWithRecord(t *testing.T, name string) (*zone.Zone, model.Record, uint32) {
	t.Helper()
	meta := model.ZoneMeta{
		Name:  name,
		Class: "IN",
		SOA:   model.SOA{MName: "ns1." + name, RName: "hostm." + name, Serial: 100, Refresh: 3600, Retry: 600, Expire: 86400, Minimum: 300},
	}
	recs := record.New()
	rr := model.Record{Name: "api." + name, Type: model.TypeA, TTL: 300, RData: "192.0.2.55"}
	if _, err := recs.Apply(model.Change{Kind: model.ChangeUpsert, Record: rr}); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	z := zone.New(name, meta, recs)
	return z, rr, z.Serial()
}

// applyChangeOnPrimary stages + commits a change through the zone, advancing
// the serial.
func applyChangeOnPrimary(t *testing.T, z *zone.Zone, ch model.Change) {
	t.Helper()
	staged, err := z.StageChange(ch)
	if err != nil {
		t.Fatalf("stage change: %v", err)
	}
	if err := staged.Commit(); err != nil {
		t.Fatalf("commit change: %v", err)
	}
}

// newSecondaryAt mirrors the primary's seed so the secondary starts in sync at
// fromSerial, then never auto-applies further primary changes.
func newSecondaryAt(t *testing.T, name string, rr model.Record, fromSerial uint32) *zone.Zone {
	t.Helper()
	meta := model.ZoneMeta{Name: name, Class: "IN", SOA: model.SOA{Serial: fromSerial, MName: "ns1." + name, RName: "hostm." + name, Refresh: 3600, Retry: 600, Expire: 86400, Minimum: 300}}
	recs := record.New()
	if _, err := recs.Apply(model.Change{Kind: model.ChangeUpsert, Record: rr}); err != nil {
		t.Fatalf("seed secondary upsert: %v", err)
	}
	return zone.New(name, meta, recs)
}

// TestIXFRDeleteStaysDeleted reproduces the reported failure: deleting a record
// on the primary must reach the secondary as a delete, not be turned back into
// an upsert that resurrects the record.
func TestIXFRDeleteStaysDeleted(t *testing.T) {
	const zoneName = "example.com"
	primary, rr, fromSerial := newPrimaryWithRecord(t, zoneName)

	// Delete the record on the primary.
	applyChangeOnPrimary(t, primary, model.Change{Kind: model.ChangeDelete, Record: rr})

	// The secondary is still at fromSerial with the record present.
	secondary := newSecondaryAt(t, zoneName, rr, fromSerial)

	svc := New(zone.NewStore(), nil)
	delta, err := svc.buildDelta(primary, fromSerial)
	if err != nil {
		t.Fatalf("buildDelta: %v", err)
	}

	// The single op must be a delete — not an upsert.
	if len(delta.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(delta.Ops))
	}
	if delta.Ops[0].Kind != model.ChangeDelete {
		t.Fatalf("expected delete op, got %v", delta.Ops[0].Kind)
	}

	if err := secondary.ApplyDelta(delta); err != nil {
		t.Fatalf("apply delta: %v", err)
	}

	// Record must be gone on the secondary.
	if recs, ok := secondary.Lookup(rr.Name, rr.Type); ok && len(recs) > 0 {
		t.Fatalf("secondary still has deleted record: %v", recs)
	}

	// Secondary serial must have advanced to the primary's current serial.
	if secondary.Serial() != primary.Serial() {
		t.Fatalf("serial not advanced: secondary=%d primary=%d", secondary.Serial(), primary.Serial())
	}
}

// TestIXFRRSerialRange verifies the delta range is Start=from, End=primary serial
// so ApplyDelta's serial-match check passes and the secondary advances.
func TestIXFRRSerialRange(t *testing.T) {
	const zoneName = "example.com"
	primary, rr, fromSerial := newPrimaryWithRecord(t, zoneName)
	applyChangeOnPrimary(t, primary, model.Change{Kind: model.ChangeUpsert, Record: model.Record{Name: "b." + zoneName, Type: model.TypeA, TTL: 300, RData: "192.0.2.66"}})

	svc := New(zone.NewStore(), nil)
	delta, err := svc.buildDelta(primary, fromSerial)
	if err != nil {
		t.Fatalf("buildDelta: %v", err)
	}

	if delta.Range.Start != fromSerial {
		t.Fatalf("Range.Start=%d, want from=%d", delta.Range.Start, fromSerial)
	}
	if delta.Range.End != primary.Serial() {
		t.Fatalf("Range.End=%d, want primary serial=%d", delta.Range.End, primary.Serial())
	}

	secondary := newSecondaryAt(t, zoneName, rr, fromSerial)
	if err := secondary.ApplyDelta(delta); err != nil {
		t.Fatalf("apply delta (should not serial-mismatch): %v", err)
	}
	if secondary.Serial() != primary.Serial() {
		t.Fatalf("secondary serial=%d, want %d", secondary.Serial(), primary.Serial())
	}
}

// TestIXFRDeleteResurrectedOldBehavior documents the pre-fix regression: if the
// op kind were upsert, the secondary would re-add the record. It asserts the
// fixed behavior holds (no resurrection) and is a guard against regressing the
// kind fix in buildDelta.
func TestIXFRDeleteNotResurrected(t *testing.T) {
	const zoneName = "example.com"
	primary, rr, fromSerial := newPrimaryWithRecord(t, zoneName)
	applyChangeOnPrimary(t, primary, model.Change{Kind: model.ChangeDelete, Record: rr})

	secondary := newSecondaryAt(t, zoneName, rr, fromSerial)
	svc := New(zone.NewStore(), nil)
	delta, err := svc.buildDelta(primary, fromSerial)
	if err != nil {
		t.Fatalf("buildDelta: %v", err)
	}
	if err := secondary.ApplyDelta(delta); err != nil {
		t.Fatalf("apply delta: %v", err)
	}
	if recs, ok := secondary.Lookup(rr.Name, rr.Type); ok && len(recs) > 0 {
		t.Fatalf("delete was resurrected as upsert on secondary: %v", recs)
	}
}
