// Package keyring seals short-lived secrets with one current key and opens
// them with the current key or any retired one, so a key can be rotated
// without invalidating what it already protected.
package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	KeySize = 32
	// MaxRetired bounds how many old keys stay loaded after rotations.
	MaxRetired = 3

	formatVersion = 1
	idSize        = 4
)

var (
	ErrNoKey   = errors.New("keyring: no encryption key configured")
	ErrInvalid = errors.New("keyring: cannot open sealed value")
)

type entry struct {
	id   [idSize]byte
	aead cipher.AEAD
}

// Keyring is safe for concurrent use: it is immutable after New.
type Keyring struct {
	current entry
	retired []entry
}

// New builds a keyring. Every key must be 32 bytes and keys must differ.
func New(current []byte, retired ...[]byte) (*Keyring, error) {
	if len(retired) > MaxRetired {
		return nil, fmt.Errorf("keyring: at most %d retired keys", MaxRetired)
	}
	first, err := newEntry(current)
	if err != nil {
		return nil, err
	}
	ring := &Keyring{current: first}
	seen := map[[idSize]byte]bool{first.id: true}
	for index, key := range retired {
		old, err := newEntry(key)
		if err != nil {
			return nil, fmt.Errorf("retired key %d: %w", index+1, err)
		}
		if seen[old.id] {
			return nil, fmt.Errorf("retired key %d repeats another key", index+1)
		}
		seen[old.id] = true
		ring.retired = append(ring.retired, old)
	}
	return ring, nil
}

func newEntry(key []byte) (entry, error) {
	if len(key) != KeySize {
		return entry{}, fmt.Errorf("keyring: key must be %d bytes", KeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return entry{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return entry{}, err
	}
	sum := sha256.Sum256(key)
	var id [idSize]byte
	copy(id[:], sum[:idSize])
	return entry{id: id, aead: aead}, nil
}

// Seal encrypts with the current key. Layout: version, key id, nonce, ciphertext.
// associated binds the value to its context and must match when opening.
func (k *Keyring) Seal(plaintext, associated []byte) ([]byte, error) {
	if k == nil {
		return nil, ErrNoKey
	}
	nonce := make([]byte, k.current.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := make([]byte, 0, 1+idSize+len(nonce)+len(plaintext)+k.current.aead.Overhead())
	sealed = append(sealed, formatVersion)
	sealed = append(sealed, k.current.id[:]...)
	sealed = append(sealed, nonce...)
	return k.current.aead.Seal(sealed, nonce, plaintext, associated), nil
}

// Open decrypts a value sealed by any key in the ring. Values written before
// key ids existed (nonce followed by ciphertext) are still accepted.
func (k *Keyring) Open(sealed, associated []byte) ([]byte, error) {
	if k == nil {
		return nil, ErrNoKey
	}
	for _, candidate := range k.entries() {
		if plain, ok := openVersioned(candidate, sealed, associated); ok {
			return plain, nil
		}
	}
	for _, candidate := range k.entries() {
		if plain, ok := openLegacy(candidate, sealed, associated); ok {
			return plain, nil
		}
	}
	return nil, ErrInvalid
}

func (k *Keyring) entries() []entry {
	return append([]entry{k.current}, k.retired...)
}

func openVersioned(candidate entry, sealed, associated []byte) ([]byte, bool) {
	header := 1 + idSize
	nonceSize := candidate.aead.NonceSize()
	if len(sealed) < header+nonceSize || sealed[0] != formatVersion || string(sealed[1:header]) != string(candidate.id[:]) {
		return nil, false
	}
	plain, err := candidate.aead.Open(nil, sealed[header:header+nonceSize], sealed[header+nonceSize:], associated)
	return plain, err == nil
}

func openLegacy(candidate entry, sealed, associated []byte) ([]byte, bool) {
	nonceSize := candidate.aead.NonceSize()
	if len(sealed) < nonceSize {
		return nil, false
	}
	plain, err := candidate.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], associated)
	return plain, err == nil
}
