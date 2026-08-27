package signer

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"strings"

	"zonedns/internal/model"
)

type RRSIG struct {
	Zone    string
	Name    string
	Type    model.RecordType
	KeyID   string
	TTL     uint32
	Expires uint32
	Incept  uint32
	Data    []byte
}

func canonicalRRSet(rrset model.RRSet) []byte {
	var payload []byte
	for _, record := range rrset.Records {
		payload = append(payload, encodeName(record.Name)...)
		var typeBytes [2]byte
		binary.BigEndian.PutUint16(typeBytes[:], uint16(record.Type))
		payload = append(payload, typeBytes[:]...)
		var classBytes [2]byte
		binary.BigEndian.PutUint16(classBytes[:], 1)
		payload = append(payload, classBytes[:]...)
		var ttlBytes [4]byte
		binary.BigEndian.PutUint32(ttlBytes[:], record.TTL)
		payload = append(payload, ttlBytes[:]...)
		payload = append(payload, encodeRData(record)...)
	}
	return payload
}

func encodeName(name string) []byte {
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	var out []byte
	for _, label := range labels {
		if label == "" {
			continue
		}
		out = append(out, byte(len(label)))
		out = append(out, strings.ToLower(label)...)
	}
	out = append(out, 0)
	return out
}

func encodeRData(record model.Record) []byte {
	return []byte(strings.ToLower(record.RData))
}

func signRRSIG(key *ecdsa.PrivateKey, rrset model.RRSet, keyID string, expires, incept uint32) (RRSIG, error) {
	var header []byte
	header = append(header, encodeName(rrset.Name)...)
	header = append(header, byte(rrset.Type), 0)
	var ttlBytes [4]byte
	binary.BigEndian.PutUint32(ttlBytes[:], 0)
	header = append(header, ttlBytes[:]...)
	var expiryBytes [4]byte
	binary.BigEndian.PutUint32(expiryBytes[:], expires)
	header = append(header, expiryBytes[:]...)
	var inceptBytes [4]byte
	binary.BigEndian.PutUint32(inceptBytes[:], incept)
	header = append(header, inceptBytes[:]...)
	header = append(header, byte(len(keyID)))
	header = append(header, keyID...)
	header = append(header, canonicalRRSet(rrset)...)
	digest := sha256.Sum256(header)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return RRSIG{}, fmt.Errorf("sign rrsig: %w", err)
	}
	return RRSIG{
		Zone:    rrset.Name,
		Name:    rrset.Name,
		Type:    rrset.Type,
		KeyID:   keyID,
		Expires: expires,
		Incept:  incept,
		Data:    signature,
	}, nil
}

func verifyRRSIG(publicKeys []*ecdsa.PublicKey, rrset model.RRSet, sig RRSIG) error {
	var header []byte
	header = append(header, encodeName(rrset.Name)...)
	header = append(header, byte(rrset.Type), 0)
	var ttlBytes [4]byte
	binary.BigEndian.PutUint32(ttlBytes[:], 0)
	header = append(header, ttlBytes[:]...)
	var expiryBytes [4]byte
	binary.BigEndian.PutUint32(expiryBytes[:], sig.Expires)
	header = append(header, expiryBytes[:]...)
	var inceptBytes [4]byte
	binary.BigEndian.PutUint32(inceptBytes[:], sig.Incept)
	header = append(header, inceptBytes[:]...)
	header = append(header, byte(len(sig.KeyID)))
	header = append(header, sig.KeyID...)
	header = append(header, canonicalRRSet(rrset)...)
	digest := sha256.Sum256(header)
	for _, public := range publicKeys {
		if ecdsa.VerifyASN1(public, digest[:], sig.Data) {
			return nil
		}
	}
	return fmt.Errorf("signature verification failed for %s %s", rrset.Name, rrset.Type)
}

func publicKeys(keys []keyWithPublic) []*ecdsa.PublicKey {
	out := make([]*ecdsa.PublicKey, 0, len(keys))
	for _, key := range keys {
		der, err := x509.MarshalPKIXPublicKey(&key.Private.PublicKey)
		if err != nil {
			continue
		}
		parsed, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			continue
		}
		if public, ok := parsed.(*ecdsa.PublicKey); ok {
			out = append(out, public)
		}
	}
	return out
}
