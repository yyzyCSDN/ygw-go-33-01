package resolver

import (
	"sort"

	"zonedns/internal/model"
)

func answerForRecords(query model.Query, records []model.Record) model.Answer {
	best := records[0]
	minTTL := best.TTL
	for _, record := range records {
		if record.TTL < minTTL {
			minTTL = record.TTL
		}
		if len(record.RData) > 0 {
			best = record
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].RData < records[j].RData })
	return model.Answer{
		Name:  query.Name,
		Type:  query.Type,
		TTL:   minTTL,
		RData: best.RData,
	}
}

func negativeAnswer(query model.Query, soa model.SOA) model.Answer {
	return model.Answer{
		Name:     query.Name,
		Type:     query.Type,
		TTL:      soa.Minimum,
		NXDomain: true,
	}
}
