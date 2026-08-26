package signer

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"zonedns/internal/keyring"
	"zonedns/internal/model"
)

type keyWithPublic = keyring.Key

type Signer struct {
	mu         sync.RWMutex
	keyring    *keyring.Keyring
	zoneName   string
	signatures map[string]RRSIG
	now        func() time.Time
	lookup     func(zoneName, name string, rtype model.RecordType) ([]model.Record, bool)
}

func New(keys *keyring.Keyring, zoneName string) *Signer {
	return &Signer{
		keyring:    keys,
		zoneName:   zoneName,
		signatures: make(map[string]RRSIG),
		now:        time.Now,
	}
}

func (s *Signer) SetLookup(lookup func(zoneName, name string, rtype model.RecordType) ([]model.Record, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookup = lookup
}

func signatureKey(name string, rtype model.RecordType) string {
	return name + "\x00" + rtype.String()
}

func (s *Signer) ResignRRSet(rrset model.RRSet) error {
	key, err := s.keyring.SigningKey()
	if err != nil {
		return fmt.Errorf("resign %s %s: %w", rrset.Name, rrset.Type, err)
	}
	now := s.now().Unix()
	expires := uint32(now + 7*24*3600)
	incept := uint32(now - 3600)
	signature, err := signRRSIG(key.Private, rrset, key.ID, expires, incept)
	if err != nil {
		return fmt.Errorf("resign %s %s: %w", rrset.Name, rrset.Type, err)
	}
	s.mu.Lock()
	s.signatures[signatureKey(rrset.Name, rrset.Type)] = signature
	s.mu.Unlock()
	return nil
}

func (s *Signer) ResignZone(rrsets []model.RRSet) error {
	for _, rrset := range rrsets {
		if err := s.ResignRRSet(rrset); err != nil {
			return err
		}
	}
	return nil
}

func (s *Signer) OnZoneUpdated(zoneName string, changedNames []string) error {
	s.mu.RLock()
	var affected []model.RRSet
	for _, name := range changedNames {
		for _, rtype := range []model.RecordType{model.TypeA, model.TypeAAAA, model.TypeCNAME, model.TypeMX, model.TypeTXT, model.TypeNS} {
			records, ok := s.lookupRecords(zoneName, name, rtype)
			if ok {
				affected = append(affected, model.RRSet{Name: name, Type: rtype, Records: records})
			}
		}
	}
	s.mu.RUnlock()
	return s.ResignZone(affected)
}

func (s *Signer) SignResponse(rrset model.RRSet) (RRSIG, error) {
	s.mu.RLock()
	signature, ok := s.signatures[signatureKey(rrset.Name, rrset.Type)]
	s.mu.RUnlock()
	if !ok {
		return RRSIG{}, fmt.Errorf("no signature for %s %s", rrset.Name, rrset.Type)
	}
	keys := s.keyring.VerifyKeys()
	if err := verifyRRSIG(publicKeys(keys), rrset, signature); err != nil {
		return RRSIG{}, err
	}
	return signature, nil
}

func (s *Signer) Verify(rrset model.RRSet, signature RRSIG) error {
	keys := s.keyring.VerifyKeys()
	if len(keys) == 0 {
		return fmt.Errorf("no verification keys available")
	}
	return verifyRRSIG(publicKeys(keys[:1]), rrset, signature)
}

func (s *Signer) Snapshot() []RRSIG {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RRSIG, 0, len(s.signatures))
	for _, sig := range s.signatures {
		out = append(out, sig)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name+out[i].Type.String() < out[j].Name+out[j].Type.String()
	})
	return out
}

func (s *Signer) lookupRecords(zoneName, name string, rtype model.RecordType) ([]model.Record, bool) {
	if s.lookup == nil {
		return nil, false
	}
	return s.lookup(zoneName, name, rtype)
}
