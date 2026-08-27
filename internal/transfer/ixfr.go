package transfer

import (
	"zonedns/internal/model"
)

func (s *Service) IXFR(name string, fromSerial uint32) (model.Delta, error) {
	z, err := s.zones.Get(name)
	if err != nil {
		return model.Delta{}, err
	}
	delta, err := s.buildDelta(z, fromSerial)
	if err != nil {
		s.Finish(name, true)
		return model.Delta{}, err
	}
	s.Finish(name, false)
	return delta, nil
}
