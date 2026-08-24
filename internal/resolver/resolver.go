package resolver

import (
	"fmt"

	"zonedns/internal/cache"
	"zonedns/internal/message"
	"zonedns/internal/metric"
	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/signer"
	"zonedns/internal/zone"
)

type Resolver struct {
	zones  *zone.Store
	cache  *cache.Cache
	signer *signer.Signer
	metric *metric.Metrics
}

func New(zones *zone.Store, responseCache *cache.Cache, sig *signer.Signer, metrics *metric.Metrics) *Resolver {
	return &Resolver{zones: zones, cache: responseCache, signer: sig, metric: metrics}
}

func (r *Resolver) Resolve(query model.Query) (model.Answer, error) {
	if result, ok := r.cache.Get(query.Name, query.Type, false); ok && !result.NXDomain && len(result.Records) > 0 {
		if r.metric != nil {
			r.metric.IncCacheHit()
		}
		// Derive the answer from the cached records with the same selection
		// rule used on a cache miss. Returning Records[0] here would diverge
		// from answerForRecords (which may pick a different record in a
		// multi-valued RRset), so the same query could answer differently
		// before and after the cache was warmed.
		return answerForRecords(query, result.Records), nil
	}
	if result, ok := r.cache.Get(query.Name, query.Type, true); ok && result.NXDomain {
		if r.metric != nil {
			r.metric.IncCacheHit()
		}
		return negativeAnswer(query, model.SOA{Minimum: result.TTL}), nil
	}
	if r.metric != nil {
		r.metric.IncCacheMiss()
	}
	return r.resolveAuthoritative(query)
}

func (r *Resolver) ResolveWire(packet []byte) ([]byte, error) {
	query, err := message.ParseQuery(packet)
	if err != nil {
		return nil, err
	}
	answer, err := r.Resolve(query)
	if err != nil {
		return nil, err
	}
	if answer.NXDomain {
		soa := r.zoneSOA(query.Name)
		return message.BuildNXDomain(query, soa)
	}
	records := []model.Record{{
		Name:  answer.Name,
		Type:  answer.Type,
		TTL:   answer.TTL,
		RData: answer.RData,
	}}
	return message.BuildAnswer(query, records)
}

func (r *Resolver) resolveAuthoritative(query model.Query) (model.Answer, error) {
	zoneName, z, err := r.findZone(query.Name)
	if err != nil {
		return model.Answer{}, err
	}
	records, ok := z.Lookup(query.Name, query.Type)
	if ok {
		rrset := model.RRSet{Name: query.Name, Type: query.Type, Records: records}
		if r.signer != nil {
			if _, err := r.signer.SignResponse(rrset); err != nil {
				return model.Answer{}, fmt.Errorf("signature validation failed for %s: %w", query.Name, err)
			}
		}
		answer := answerForRecords(query, records)
		r.cache.Put(query.Name, query.Type, false, records, answer.TTL)
		return answer, nil
	}
	soa := r.zoneSOA(zoneName)
	r.cache.StoreNegative(query, soa)
	return negativeAnswer(query, soa), nil
}

func (r *Resolver) findZone(name string) (string, *zone.Zone, error) {
	// Normalize the query name before matching it against zone names: DNS is
	// case-insensitive, and matching on the raw query would let a mixed-case
	// query such as "WWW.EXAMPLE.COM" slip past the authoritative zone for
	// "example.com" and resolve as NXDOMAIN.
	canon := record.CanonicalName(name)
	names := r.zones.List()
	best := ""
	for _, candidate := range names {
		c := record.CanonicalName(candidate)
		if canon == c || len(canon) > len(c) && canon[len(canon)-len(c)-1] == '.' && canon[len(canon)-len(c):] == c {
			if len(c) > len(best) {
				best = candidate
			}
		}
	}
	if best == "" {
		return "", nil, fmt.Errorf("no authoritative zone for %q", name)
	}
	z, err := r.zones.Get(best)
	if err != nil {
		return "", nil, err
	}
	return best, z, nil
}

func (r *Resolver) zoneSOA(name string) model.SOA {
	_, z, err := r.findZone(name)
	if err != nil {
		return model.SOA{Minimum: 300}
	}
	return z.Meta().SOA
}
