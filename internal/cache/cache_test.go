package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"zonedns/internal/model"
)

func sampleRecord(rdata string) []model.Record {
	return []model.Record{{
		Name:  "example.com.",
		Type:  model.TypeA,
		TTL:   60,
		RData: rdata,
	}}
}

// A sweep running concurrently with Get must never tear an in-flight read: a
// Get that observes the entry must return its full data, not nil/empty records.
func TestGetConcurrentSweepNoTornRead(t *testing.T) {
	c := New(1024)

	// Seed the cache with a known entry.
	const rdata = "203.0.113.1"
	c.Put("example.com.", model.TypeA, false, sampleRecord(rdata), 60)

	now := time.Now()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Sweeper: repeatedly sweeps an expiry time that alternately covers and
	// misses the entry, racing deletes against reads.
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			sweepNow := now.Add(10 * time.Minute) // entry expired relative to this
			if i%2 == 0 {
				sweepNow = now // entry not yet expired
			}
			c.SweepExpired(sweepNow)
			// Re-populate so the reader has something to find again.
			c.Put("example.com.", model.TypeA, false, sampleRecord(rdata), 60)
			i++
		}
	}()

	// Reader: every hit must carry the expected record data.
	for i := 0; i < 20000; i++ {
		res, ok := c.Get("example.com.", model.TypeA, false)
		if ok {
			if len(res.Records) == 0 {
				close(stop)
				t.Fatalf("torn read: Get returned ok=true but empty records at iter %d", i)
			}
			if res.Records[0].RData != rdata {
				close(stop)
				t.Fatalf("torn read: Get returned ok=true but RData=%q want %q at iter %d",
					res.Records[0].RData, rdata, i)
			}
		}
	}
	close(stop)
	wg.Wait()
}

// Sanity: the negative-caching path and basic expiry still behave.
func TestSweepExpiredRemovesOnlyExpired(t *testing.T) {
	c := New(1024)
	t0 := time.Now()
	c.now = func() time.Time { return t0 }

	c.Put("fresh.", model.TypeA, false, sampleRecord("1.1.1.1"), 3600)
	c.Put("stale.", model.TypeA, false, sampleRecord("2.2.2.2"), 60)

	removed := c.SweepExpired(t0.Add(2 * time.Minute))
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	if _, ok := c.Get("fresh.", model.TypeA, false); !ok {
		t.Fatal("fresh entry should remain")
	}
	if _, ok := c.Get("stale.", model.TypeA, false); ok {
		t.Fatal("stale entry should be gone")
	}
}

// Concurrent Get/Put/Sweep must be race-free under -race.
func TestConcurrentGetPutSweepRace(t *testing.T) {
	c := New(256)
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func(g int) {
			defer wg.Done()
			for n := 0; n < 4000; n++ {
				name := fmt.Sprintf("host%d.", g%4)
				c.Get(name, model.TypeA, false)
			}
		}(i)
		go func(g int) {
			defer wg.Done()
			for n := 0; n < 4000; n++ {
				name := fmt.Sprintf("host%d.", g%4)
				c.Put(name, model.TypeA, false, sampleRecord("203.0.113.1"), 60)
			}
		}(i)
		go func() {
			defer wg.Done()
			for n := 0; n < 4000; n++ {
				c.SweepExpired(now.Add(time.Duration(n%3) * time.Minute))
			}
		}()
	}
	wg.Wait()
}
