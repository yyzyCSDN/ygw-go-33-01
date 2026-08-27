package keyring

import (
	"fmt"
	"sort"
	"time"
)

func (k *Keyring) Rollover(newKey Key, activeFrom time.Time) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, exists := k.keys[newKey.ID]; exists {
		return fmt.Errorf("rollover key %q already exists", newKey.ID)
	}
	now := time.Now().UTC()
	if activeFrom.Before(now) {
		activeFrom = now
	}
	newKey.ActiveFrom = activeFrom
	newKey.ActiveUntil = now.Add(365 * 24 * time.Hour)
	for _, id := range k.order {
		old := k.keys[id]
		if old.ActiveUntil.IsZero() {
			old.ActiveUntil = now.Add(30 * 24 * time.Hour)
		}
		k.keys[id] = old
	}
	k.keys[newKey.ID] = newKey
	k.order = append(k.order, newKey.ID)
	k.activeKeyID = newKey.ID
	k.verifyKeyIDs = k.verificationSetLocked(now)
	return nil
}

func (k *Keyring) verificationSetLocked(now time.Time) []string {
	ids := make([]string, 0, len(k.order))
	for _, id := range k.order {
		key := k.keys[id]
		if !key.ActiveFrom.After(now) && (key.ActiveUntil.IsZero() || !key.ActiveUntil.Before(now)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
