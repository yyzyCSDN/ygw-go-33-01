package transfer

import (
	"fmt"
	"sync"

	"zonedns/internal/model"
	"zonedns/internal/zone"
)

type ZoneLike interface {
	Name() string
	Serial() uint32
	ChangesSince(from uint32) ([]model.Change, bool)
	DeltaRangeFor(from uint32) model.SerialRange
}

type Service struct {
	mu          sync.Mutex
	zones       *zone.Store
	secondaries *zone.Store
	states      map[string]model.TransferState
	notifier    Notifier
}

func New(zones *zone.Store, notifier Notifier) *Service {
	return &Service{
		zones:    zones,
		states:   make(map[string]model.TransferState),
		notifier: notifier,
	}
}

func (s *Service) SetSecondaries(secondaries *zone.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secondaries = secondaries
}

func (s *Service) Secondary(name string) (*zone.Zone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.secondaries == nil {
		return nil, fmt.Errorf("secondary store not configured")
	}
	return s.secondaries.Get(name)
}

func (s *Service) ApplyAXFR(name string, snapshot model.ZoneSnapshot) error {
	secondary, err := s.Secondary(name)
	if err != nil {
		return err
	}
	if err := secondary.ApplyFull(snapshot); err != nil {
		s.Finish(name, true)
		return fmt.Errorf("apply axfr to secondary: %w", err)
	}
	s.Finish(name, false)
	return nil
}

func (s *Service) ApplyIXFR(name string, delta model.Delta) error {
	secondary, err := s.Secondary(name)
	if err != nil {
		return err
	}
	if err := secondary.ApplyDelta(delta); err != nil {
		s.Finish(name, true)
		return fmt.Errorf("apply ixfr to secondary: %w", err)
	}
	s.Finish(name, false)
	return nil
}

func (s *Service) State(name string) model.TransferState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[name]
	if !ok {
		return model.TransferIdle
	}
	return state
}

func (s *Service) StartTransfer(name string) (model.TransferState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.states[name]
	if current == model.TransferInProgress {
		return current, fmt.Errorf("transfer already in progress for %q", name)
	}
	if current == model.TransferComplete {
		s.states[name] = model.TransferInProgress
		return model.TransferInProgress, nil
	}
	if current == model.TransferFailed {
		s.states[name] = model.TransferInProgress
		return model.TransferInProgress, nil
	}
	s.states[name] = model.TransferInProgress
	return model.TransferInProgress, nil
}

func (s *Service) Finish(name string, failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if failed {
		s.states[name] = model.TransferFailed
		return
	}
	s.states[name] = model.TransferComplete
}

func (s *Service) NotifyZone(name string, serial uint32) error {
	if s.notifier == nil {
		return nil
	}
	if err := s.notifier.Notify(model.NotifyEvent{Zone: name, Serial: serial}); err != nil {
		s.Finish(name, true)
		return fmt.Errorf("notify %q: %w", name, err)
	}
	s.Finish(name, false)
	return nil
}

func (s *Service) buildDelta(z ZoneLike, fromSerial uint32) (model.Delta, error) {
	current := z.Serial()
	if fromSerial == current {
		return model.Delta{Zone: z.Name(), Range: z.DeltaRangeFor(fromSerial)}, nil
	}
	changes, ok := z.ChangesSince(fromSerial)
	if !ok {
		return model.Delta{}, fmt.Errorf("no change history available from serial %d", fromSerial)
	}
	ops := make([]model.DeltaOp, 0, len(changes))
	for _, ch := range changes {
		ops = append(ops, model.DeltaOp{Kind: model.ChangeUpsert, Record: ch.Record})
	}
	return model.Delta{
		Zone:  z.Name(),
		Range: z.DeltaRangeFor(fromSerial),
		Ops:   ops,
	}, nil
}
