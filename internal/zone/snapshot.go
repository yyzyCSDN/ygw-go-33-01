package zone

import (
	"fmt"

	"zonedns/internal/model"
)

func (z *Zone) Snapshot() (model.ZoneSnapshot, error) {
	z.mu.RLock()
	serial := z.serial
	meta := z.meta
	records := append([]model.Record(nil), z.records.All()...)
	z.mu.RUnlock()
	return model.ZoneSnapshot{
		Name:    z.name,
		Meta:    meta,
		Serial:  serial,
		Records: records,
	}, nil
}

func (z *Zone) SnapshotAt(requested uint32) (model.ZoneSnapshot, error) {
	snapshot, err := z.Snapshot()
	if err != nil {
		return model.ZoneSnapshot{}, err
	}
	if requested != 0 && requested != snapshot.Serial {
		return model.ZoneSnapshot{}, fmt.Errorf("serial changed during snapshot: requested=%d actual=%d", requested, snapshot.Serial)
	}
	return snapshot, nil
}

func (z *Zone) ApplyFull(snapshot model.ZoneSnapshot) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	rebuilt := z.records.Snapshot()
	for name := range rebuilt {
		for rtype := range rebuilt[name] {
			delete(rebuilt[name], rtype)
		}
	}
	z.records.Restore(rebuilt)
	for _, r := range snapshot.Records {
		if _, err := z.records.Apply(model.Change{Kind: model.ChangeUpsert, Record: r}); err != nil {
			return fmt.Errorf("apply axfr record: %w", err)
		}
	}
	z.serial = snapshot.Serial
	z.meta = snapshot.Meta
	z.version++
	z.history = nil
	return nil
}
