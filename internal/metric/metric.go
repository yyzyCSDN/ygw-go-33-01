package metric

import (
	"sync/atomic"
)

type Metrics struct {
	queries       atomic.Int64
	cacheHits     atomic.Int64
	cacheMisses   atomic.Int64
	transfers     atomic.Int64
	signFailures  atomic.Int64
	updates       atomic.Int64
	notifySent    atomic.Int64
	journalWrites atomic.Int64
}

func New() *Metrics {
	return &Metrics{}
}

func (m *Metrics) IncQuery() { m.queries.Add(1) }

func (m *Metrics) IncCacheHit() { m.cacheHits.Add(1) }

func (m *Metrics) IncCacheMiss() { m.cacheMisses.Add(1) }

func (m *Metrics) IncTransfer() { m.transfers.Add(1) }

func (m *Metrics) IncSignFailure() { m.signFailures.Add(1) }

func (m *Metrics) IncUpdate() { m.updates.Add(1) }

func (m *Metrics) IncNotify() { m.notifySent.Add(1) }

func (m *Metrics) IncJournalWrite() { m.journalWrites.Add(1) }

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"queries":        m.queries.Load(),
		"cache_hits":     m.cacheHits.Load(),
		"cache_misses":   m.cacheMisses.Load(),
		"transfers":      m.transfers.Load(),
		"sign_failures":  m.signFailures.Load(),
		"updates":        m.updates.Load(),
		"notify_sent":    m.notifySent.Load(),
		"journal_writes": m.journalWrites.Load(),
	}
}
