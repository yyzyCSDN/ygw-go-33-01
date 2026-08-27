package cache

import (
	"fmt"
	"sync"
	"testing"

	"zonedns/internal/model"
)

func TestCacheEvictDoesNotBreakReads(t *testing.T) {
	c := New(64)
	records := []model.Record{{Name: "www.example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.10"}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	bad := make(chan string, 4)
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 500; i++ {
				c.Put("www.example.com", model.TypeA, false, records, 0)
				if result, ok := c.Get("www.example.com", model.TypeA, false); ok {
					if len(result.Records) == 0 {
						select {
						case bad <- fmt.Sprintf("empty result on live entry at iteration %d", i):
						default:
						}
						return
					}
				}
			}
		}()
	}
	close(start)
	for i := 0; i < 200; i++ {
		c.SweepExpired(c.now())
	}
	wg.Wait()
	select {
	case message := <-bad:
		t.Fatalf("cache read broken: %s", message)
	default:
	}
}
