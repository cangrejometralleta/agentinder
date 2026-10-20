package keyring

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"testing"
)

func key(fill byte) []byte { return bytes.Repeat([]byte{fill}, KeySize) }

func mustRing(t *testing.T, current []byte, retired ...[]byte) *Keyring {
	t.Helper()
	ring, err := New(current, retired...)
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func TestSealThenOpenRoundTrip(t *testing.T) {
	ring := mustRing(t, key(1))
	sealed, err := ring.Seal([]byte("bearer"), []byte("context"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("bearer")) {
		t.Error("sealed value leaks the plaintext")
	}
	plain, err := ring.Open(sealed, []byte("context"))
	if err != nil || string(plain) != "bearer" {
		t.Fatalf("round trip failed: %q %v", plain, err)
	}
}

func TestOpenRejectsOtherContextAndTampering(t *testing.T) {
	ring := mustRing(t, key(1))
	sealed, _ := ring.Seal([]byte("bearer"), []byte("context"))
	if _, err := ring.Open(sealed, []byte("another")); err == nil {
		t.Error("a value must not open under another context")
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := ring.Open(sealed, []byte("context")); err == nil {
		t.Error("a tampered value must not open")
	}
}

func TestRotationKeepsOldValuesReadable(t *testing.T) {
	oldRing := mustRing(t, key(1))
	sealedByOld, _ := oldRing.Seal([]byte("before rotation"), []byte("c"))

	rotated := mustRing(t, key(2), key(1))
	if plain, err := rotated.Open(sealedByOld, []byte("c")); err != nil || string(plain) != "before rotation" {
		t.Fatalf("a retired key must still open old values: %q %v", plain, err)
	}
	sealedByNew, _ := rotated.Seal([]byte("after rotation"), []byte("c"))
	if _, err := oldRing.Open(sealedByNew, []byte("c")); err == nil {
		t.Error("new values must be sealed with the new key only")
	}

	dropped := mustRing(t, key(2))
	if _, err := dropped.Open(sealedByOld, []byte("c")); err == nil {
		t.Error("once the old key is dropped its values stop opening")
	}
}

func TestOpensValuesWrittenBeforeKeyIds(t *testing.T) {
	block, _ := aes.NewCipher(key(1))
	aead, _ := cipher.NewGCM(block)
	for attempt := 0; attempt < 600; attempt++ { // covers nonces that start with the version byte
		nonce := make([]byte, aead.NonceSize())
		_, _ = rand.Read(nonce)
		legacy := aead.Seal(nonce, nonce, []byte("legacy"), []byte("c"))
		ring := mustRing(t, key(2), key(1))
		plain, err := ring.Open(legacy, []byte("c"))
		if err != nil || string(plain) != "legacy" {
			t.Fatalf("attempt %d: legacy value did not open: %v", attempt, err)
		}
	}
}

func TestNewValidatesKeys(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("a short key must be refused")
	}
	if _, err := New(key(1), key(1)); err == nil {
		t.Error("a retired key equal to the current one must be refused")
	}
	if _, err := New(key(1), key(2), key(2)); err == nil {
		t.Error("repeated retired keys must be refused")
	}
	if _, err := New(key(1), key(2), key(3), key(4), key(5)); err == nil {
		t.Error("too many retired keys must be refused")
	}
}

func TestNilKeyringFailsClosed(t *testing.T) {
	var ring *Keyring
	if _, err := ring.Seal([]byte("x"), nil); err != ErrNoKey {
		t.Errorf("want ErrNoKey, got %v", err)
	}
	if _, err := ring.Open([]byte("x"), nil); err != ErrNoKey {
		t.Errorf("want ErrNoKey, got %v", err)
	}
}
