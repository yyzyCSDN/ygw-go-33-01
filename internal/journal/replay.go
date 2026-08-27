package journal

import "fmt"

type ReplayApplier interface {
	ApplyReplayed(zone string, serial uint32, change Change) error
}

func Replay(durability Durability, applier ReplayApplier) error {
	entries, err := durability.Replay()
	if err != nil {
		return fmt.Errorf("replay journal: %w", err)
	}
	for _, entry := range entries {
		if err := applier.ApplyReplayed(entry.Zone, entry.Serial, entry.Change); err != nil {
			return fmt.Errorf("replay entry for %s: %w", entry.Zone, err)
		}
	}
	return nil
}
