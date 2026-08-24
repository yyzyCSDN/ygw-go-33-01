package record

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"zonedns/internal/model"
)

func CanonicalName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSuffix(lower, ".")
}

type Store struct {
	mu      sync.RWMutex
	records map[string]map[model.RecordType][]model.Record
}

func New() *Store {
	return &Store{records: make(map[string]map[model.RecordType][]model.Record)}
}

func (s *Store) Apply(ch model.Change) (model.Record, error) {
	if err := ValidateRecord(ch.Record); err != nil {
		return model.Record{}, err
	}
	name := CanonicalName(ch.Record.Name)
	record := ch.Record
	record.Name = name
	s.mu.Lock()
	defer s.mu.Unlock()
	byType := s.records[name]
	if byType == nil {
		byType = make(map[model.RecordType][]model.Record)
		s.records[name] = byType
	}
	kept := byType[ch.Record.Type][:0]
	for _, existing := range byType[ch.Record.Type] {
		if existing.RData != record.RData {
			kept = append(kept, existing)
		}
	}
	if ch.Kind == model.ChangeDelete {
		if len(kept) == 0 {
			delete(byType, ch.Record.Type)
		} else {
			byType[ch.Record.Type] = kept
		}
		if len(byType) == 0 {
			delete(s.records, name)
		}
		return record, nil
	}
	if ch.Kind != model.ChangeUpsert {
		return model.Record{}, fmt.Errorf("unknown change kind %v", ch.Kind)
	}
	byType[ch.Record.Type] = append(kept, record)
	s.records[name] = byType
	return ch.Record, nil
}

func (s *Store) Lookup(name string, rtype model.RecordType) ([]model.Record, bool) {
	key := CanonicalName(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	byType, ok := s.records[key]
	if !ok {
		return nil, false
	}
	records := byType[rtype]
	if len(records) == 0 {
		return nil, false
	}
	copied := make([]model.Record, len(records))
	copy(copied, records)
	return copied, true
}

func (s *Store) All() []model.Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Record
	for _, byType := range s.records {
		for _, records := range byType {
			out = append(out, records...)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].RData < out[j].RData
	})
	return out
}

func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.records))
	for name := range s.records {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Store) Snapshot() map[string]map[model.RecordType][]model.Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make(map[string]map[model.RecordType][]model.Record, len(s.records))
	for name, byType := range s.records {
		inner := make(map[model.RecordType][]model.Record, len(byType))
		for rtype, records := range byType {
			inner[rtype] = append([]model.Record(nil), records...)
		}
		copied[name] = inner
	}
	return copied
}

func (s *Store) Restore(snapshot map[string]map[model.RecordType][]model.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = snapshot
}
