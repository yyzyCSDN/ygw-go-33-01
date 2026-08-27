package control

import (
	"fmt"
	"sort"

	"zonedns/internal/metric"
	"zonedns/internal/model"
	"zonedns/internal/signer"
	"zonedns/internal/transfer"
	"zonedns/internal/zone"
)

type Service struct {
	zones    *zone.Store
	signer   *signer.Signer
	transfer *transfer.Service
	metrics  *metric.Metrics
}

func New(zones *zone.Store, sig *signer.Signer, transfers *transfer.Service, metrics *metric.Metrics) *Service {
	return &Service{
		zones:    zones,
		signer:   sig,
		transfer: transfers,
		metrics:  metrics,
	}
}

func (s *Service) Reload(zoneName string) error {
	z, err := s.zones.Get(zoneName)
	if err != nil {
		return err
	}
	rrsets := rrsetsOf(z)
	if err := s.signer.ResignZone(rrsets); err != nil {
		return fmt.Errorf("reload resign %q: %w", zoneName, err)
	}
	return nil
}

func (s *Service) TriggerTransfer(zoneName string) (model.TransferState, error) {
	state, err := s.transfer.StartTransfer(zoneName)
	if err != nil {
		return state, err
	}
	s.metrics.IncTransfer()
	s.transfer.Finish(zoneName, false)
	return model.TransferComplete, nil
}

func (s *Service) Status() map[string]map[string]string {
	out := make(map[string]map[string]string)
	for _, name := range s.zones.List() {
		z, err := s.zones.Get(name)
		if err != nil {
			continue
		}
		out[name] = map[string]string{
			"serial":   fmt.Sprintf("%d", z.Serial()),
			"version":  fmt.Sprintf("%d", z.Version()),
			"transfer": s.transfer.State(name).String(),
		}
	}
	return out
}

func rrsetsOf(z *zone.Zone) []model.RRSet {
	names := z.Records().Names()
	var rrsets []model.RRSet
	for _, name := range names {
		for _, rtype := range []model.RecordType{model.TypeA, model.TypeAAAA, model.TypeCNAME, model.TypeMX, model.TypeTXT, model.TypeNS} {
			records, ok := z.Lookup(name, rtype)
			if ok {
				rrsets = append(rrsets, model.RRSet{Name: name, Type: rtype, Records: records})
			}
		}
	}
	sort.Slice(rrsets, func(i, j int) bool {
		if rrsets[i].Name != rrsets[j].Name {
			return rrsets[i].Name < rrsets[j].Name
		}
		return rrsets[i].Type < rrsets[j].Type
	})
	return rrsets
}
