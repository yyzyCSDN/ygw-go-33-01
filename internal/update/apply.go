package update

import (
	"fmt"
	"strings"

	"zonedns/internal/journal"
	"zonedns/internal/model"
	"zonedns/internal/record"
)

func validateChange(zoneName string, ch model.Change) error {
	if ch.Kind != model.ChangeUpsert && ch.Kind != model.ChangeDelete {
		return fmt.Errorf("unsupported change kind %v", ch.Kind)
	}
	if err := record.ValidateRecord(ch.Record); err != nil {
		return fmt.Errorf("invalid record: %w", err)
	}
	name := record.CanonicalName(ch.Record.Name)
	zone := record.CanonicalName(zoneName)
	if name != zone && !strings.HasSuffix(name, "."+zone) {
		return fmt.Errorf("record %q is outside zone %q", name, zoneName)
	}
	if ch.Record.Type == model.TypeCNAME && ch.Kind == model.ChangeUpsert {
		return fmt.Errorf("CNAME records cannot be created through dynamic updates")
	}
	return nil
}

func toJournalChange(ch model.Change) journal.Change {
	return journal.Change{
		Kind: ch.Kind.String(),
		Record: journal.Record{
			Name:  ch.Record.Name,
			Type:  ch.Record.Type.String(),
			TTL:   ch.Record.TTL,
			RData: ch.Record.RData,
		},
	}
}
