package cache

import (
	"testing"

	"zonedns/internal/model"
)

func TestNegativeCacheTtlUsesCorrectSource(t *testing.T) {
	c := New(64)
	soa := model.SOA{
		MName:   "ns1.example.com",
		RName:   "hostmaster.example.com",
		Serial:  100,
		Minimum: 60,
	}
	query := model.Query{Name: "missing.example.com", Type: model.TypeA}
	c.StoreNegative(query, soa)
	result, ok := c.Get(query.Name, query.Type, true)
	if !ok {
		t.Fatal("negative entry not cached")
	}
	if result.TTL != 60 {
		t.Fatalf("negative cache TTL %d does not follow SOA minimum 60", result.TTL)
	}
}
