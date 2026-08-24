package zone

import (
	"fmt"
	"sync"

	"zonedns/internal/model"
	"zonedns/internal/record"
)

type ResignHook func(zoneName string, changedNames []string) error

type Zone struct {
	mu       sync.RWMutex
	name     string
	meta     model.ZoneMeta
	records  *record.Store
	serial   uint32
	version  uint64
	history  []historyEntry
	hook     ResignHook
	notifyCb func(name string, serial uint32)
}

type historyEntry struct {
	serial uint32
	change model.Change
}

func New(name string, meta model.ZoneMeta, records *record.Store) *Zone {
	return &Zone{
		name:    name,
		meta:    meta,
		records: records,
		serial:  meta.SOA.Serial,
	}
}

func (z *Zone) SetResignHook(hook ResignHook) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.hook = hook
}

func (z *Zone) SetNotifyCallback(cb func(name string, serial uint32)) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.notifyCb = cb
}

func (z *Zone) Name() string {
	return z.name
}

func (z *Zone) Meta() model.ZoneMeta {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.meta
}

func (z *Zone) Serial() uint32 {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.serial
}

func (z *Zone) Version() uint64 {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.version
}

func (z *Zone) Lookup(name string, rtype model.RecordType) ([]model.Record, bool) {
	return z.records.Lookup(name, rtype)
}

func (z *Zone) Records() *record.Store {
	return z.records
}

func (z *Zone) StageChange(ch model.Change) (*StagedChange, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	before := z.records.Snapshot()
	applied, err := z.records.Apply(ch)
	if err != nil {
		return nil, fmt.Errorf("apply record change: %w", err)
	}
	serialBefore := z.serial
	versionBefore := z.version
	return &StagedChange{
		zone:          z,
		change:        ch,
		applied:       applied,
		before:        before,
		serialBefore:  serialBefore,
		versionBefore: versionBefore,
		committed:     false,
	}, nil
}

type StagedChange struct {
	zone          *Zone
	change        model.Change
	applied       model.Record
	before        map[string]map[model.RecordType][]model.Record
	serialBefore  uint32
	versionBefore uint64
	committed     bool
}

func (s *StagedChange) Applied() model.Record {
	return s.applied
}

func (s *StagedChange) Commit() error {
	if s.committed {
		return fmt.Errorf("staged change already committed")
	}
	s.zone.mu.Lock()
	s.zone.version++
	s.zone.history = append(s.zone.history, historyEntry{serial: s.zone.serial, change: s.change})
	changed := []string{s.applied.Name}
	hook := s.zone.hook
	notify := s.zone.notifyCb
	serial := s.zone.serial
	s.zone.mu.Unlock()
	if hook != nil {
		if err := hook(s.zone.name, changed); err != nil {
			return fmt.Errorf("resign affected rrsets: %w", err)
		}
	}
	if notify != nil {
		notify(s.zone.name, serial)
	}
	s.committed = true
	return nil
}

func (s *StagedChange) Rollback() {
	if s.committed {
		return
	}
	s.zone.mu.Lock()
	defer s.zone.mu.Unlock()
	s.zone.records.Restore(s.before)
	s.zone.serial = s.serialBefore
	s.zone.version = s.versionBefore
}


func (z *Zone) ChangesSince(from uint32) ([]model.Change, bool) {
	z.mu.RLock()
	defer z.mu.RUnlock()
	if len(z.history) == 0 {
		return nil, false
	}
	if z.serial == from {
		return nil, true
	}
	var changes []model.Change
	for _, entry := range z.history {
		if entry.serial > from {
			changes = append(changes, entry.change)
		}
	}
	return changes, true
}

func (z *Zone) DeltaRangeFor(from uint32) model.SerialRange {
	return RangeBetween(from, z.Serial())
}

func (z *Zone) ApplyDelta(delta model.Delta) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if delta.Range.Start != z.serial {
		return fmt.Errorf("serial mismatch: local=%d delta=[%d,%d)", z.serial, delta.Range.Start, delta.Range.End)
	}
	for _, op := range delta.Ops {
		if _, err := z.records.Apply(model.Change{Kind: op.Kind, Record: op.Record}); err != nil {
			return fmt.Errorf("apply delta op: %w", err)
		}
	}
	z.serial = delta.Range.End
	z.meta.SOA.Serial = z.serial
	z.version++
	z.history = append(z.history, historyEntry{serial: z.serial, change: model.Change{Kind: model.ChangeUpsert, Record: model.Record{Name: delta.Zone, Type: model.TypeSOA, TTL: 0, RData: fmt.Sprintf("%d", z.serial)}}})
	return nil
}
