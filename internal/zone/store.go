package zone

import (
	"fmt"
	"sort"
	"sync"
)

type Store struct {
	mu    sync.RWMutex
	zones map[string]*Zone
}

func NewStore() *Store {
	return &Store{zones: make(map[string]*Zone)}
}

func (s *Store) Put(z *Zone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones[z.Name()] = z
}

func (s *Store) Get(name string) (*Zone, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	z, ok := s.zones[name]
	if !ok {
		return nil, fmt.Errorf("zone %q not found", name)
	}
	return z, nil
}

func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.zones))
	for name := range s.zones {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Store) MustGet(name string) *Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.zones[name]
}
