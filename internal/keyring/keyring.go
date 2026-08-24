package keyring

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Key struct {
	ID          string
	CreatedAt   time.Time
	ActiveFrom  time.Time
	ActiveUntil time.Time
	Private     *ecdsa.PrivateKey
}

func (k Key) PublicPEM() ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(&k.Private.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

type Keyring struct {
	mu           sync.RWMutex
	keys         map[string]Key
	order        []string
	activeKeyID  string
	verifyKeyIDs []string
}

func New() *Keyring {
	return &Keyring{keys: make(map[string]Key)}
}

func Generate(id string) (Key, error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Key{}, fmt.Errorf("generate key: %w", err)
	}
	return Key{
		ID:          id,
		CreatedAt:   time.Now().UTC(),
		ActiveFrom:  time.Now().UTC(),
		ActiveUntil: time.Now().UTC().Add(365 * 24 * time.Hour),
		Private:     private,
	}, nil
}

func (k *Keyring) Add(key Key) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, exists := k.keys[key.ID]; exists {
		return fmt.Errorf("key %q already exists", key.ID)
	}
	k.keys[key.ID] = key
	k.order = append(k.order, key.ID)
	k.verifyKeyIDs = append(k.verifyKeyIDs, key.ID)
	if k.activeKeyID == "" {
		k.activeKeyID = key.ID
	}
	return nil
}

func (k *Keyring) SigningKey() (Key, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.activeKeyID == "" {
		return Key{}, fmt.Errorf("no active signing key")
	}
	key, ok := k.keys[k.activeKeyID]
	if !ok {
		return Key{}, fmt.Errorf("active key %q missing", k.activeKeyID)
	}
	return key, nil
}

func (k *Keyring) VerifyKeys() []Key {
	k.mu.RLock()
	defer k.mu.RUnlock()
	keys := make([]Key, 0, len(k.verifyKeyIDs))
	for _, id := range k.verifyKeyIDs {
		if key, ok := k.keys[id]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func (k *Keyring) ActiveKeyID() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.activeKeyID
}

func (k *Keyring) KeyIDs() []string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return append([]string(nil), k.order...)
}

func (k *Keyring) VerifyKeyIDs() []string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	ids := append([]string(nil), k.verifyKeyIDs...)
	sort.Strings(ids)
	return ids
}
