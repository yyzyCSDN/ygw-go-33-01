package update

import (
	"fmt"
	"sync"

	"zonedns/internal/journal"
	"zonedns/internal/metric"
	"zonedns/internal/model"
	"zonedns/internal/transfer"
	"zonedns/internal/zone"
)

type Service struct {
	mu       sync.Mutex
	zones    *zone.Store
	journal  journal.Durability
	transfer *transfer.Service
	metrics  *metric.Metrics
}

func New(zones *zone.Store, durability journal.Durability, transfers *transfer.Service, metrics *metric.Metrics) *Service {
	return &Service{
		zones:    zones,
		journal:  durability,
		transfer: transfers,
		metrics:  metrics,
	}
}

func (s *Service) Apply(zoneName string, ch model.Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateChange(zoneName, ch); err != nil {
		return fmt.Errorf("reject dynamic update: %w", err)
	}
	z, err := s.zones.Get(zoneName)
	if err != nil {
		return err
	}
	staged, err := z.StageChange(ch)
	if err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}
	entry := journal.Entry{
		Zone:   zoneName,
		Serial: z.Serial(),
		Change: toJournalChange(ch),
	}
	if err := s.transfer.NotifyZone(zoneName, z.Serial()); err != nil {
		return fmt.Errorf("apply succeeded but notify failed: %w", err)
	}
	if err := s.journal.Append(entry); err != nil {
		staged.Rollback()
		return fmt.Errorf("apply failed: journal append: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncJournalWrite()
	}
	if err := staged.Commit(); err != nil {
		staged.Rollback()
		return fmt.Errorf("apply failed: commit: %w", err)
	}
	return nil
}

func (s *Service) ApplyReplayed(zoneName string, serial uint32, change journal.Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, err := s.zones.Get(zoneName)
	if err != nil {
		return err
	}
	ch := model.Change{
		Kind: model.ChangeUpsert,
		Record: model.Record{
			Name:  change.Record.Name,
			TTL:   change.Record.TTL,
			RData: change.Record.RData,
		},
	}
	if change.Kind == "delete" {
		ch.Kind = model.ChangeDelete
	}
	switch change.Record.Type {
	case "A":
		ch.Record.Type = model.TypeA
	case "AAAA":
		ch.Record.Type = model.TypeAAAA
	case "TXT":
		ch.Record.Type = model.TypeTXT
	case "MX":
		ch.Record.Type = model.TypeMX
	case "NS":
		ch.Record.Type = model.TypeNS
	default:
		return fmt.Errorf("unsupported replayed record type %q", change.Record.Type)
	}
	staged, err := z.StageChange(ch)
	if err != nil {
		return fmt.Errorf("replay apply: %w", err)
	}
	if err := staged.Commit(); err != nil {
		staged.Rollback()
		return fmt.Errorf("replay commit: %w", err)
	}
	return nil
}
