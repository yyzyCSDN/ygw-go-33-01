package keyring

import (
	"testing"
	"time"
)

func TestVerificationAcceptsRolledKey(t *testing.T) {
	k := New()
	k1, err := Generate("k1")
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Add(k1); err != nil {
		t.Fatal(err)
	}
	k2, err := Generate("k2")
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Rollover(k2, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if k.ActiveKeyID() != "k2" {
		t.Fatalf("active key %q != k2", k.ActiveKeyID())
	}
	ids := make(map[string]bool)
	for _, id := range k.VerifyKeyIDs() {
		ids[id] = true
	}
	if !ids["k1"] || !ids["k2"] {
		t.Fatalf("verification keys %v do not cover both keys during rollover", k.VerifyKeyIDs())
	}
}
