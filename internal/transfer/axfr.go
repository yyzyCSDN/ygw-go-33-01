package transfer

import (
	"fmt"

	"zonedns/internal/model"
)

func (s *Service) AXFR(name string) (model.ZoneSnapshot, error) {
	z, err := s.zones.Get(name)
	if err != nil {
		return model.ZoneSnapshot{}, err
	}
	if cached, ok := s.snapshotCache[name]; ok {
		s.Finish(name, false)
		return cached, nil
	}
	current := z.Serial()
	snapshot, err := z.SnapshotAt(current)
	if err != nil {
		s.Finish(name, true)
		return model.ZoneSnapshot{}, fmt.Errorf("axfr snapshot: %w", err)
	}
	s.Finish(name, false)
	return snapshot, nil
}
