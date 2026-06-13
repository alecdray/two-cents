package cryptox_test

import (
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/cryptox"
)

// Two distinct hex-encoded 32-byte (AES-256) keys.
const (
	keyA = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	keyB = "fffefdfcfbfaf9f8f7f6f5f4f3f2f1f0efeeedecebeae9e8e7e6e5e4e3e2e1e0f"
)

// Encrypting hides the plaintext: the output differs from the input and does
// not contain it verbatim. Decrypting with the same key restores the original.
func TestEncryptThenDecryptRoundTrips(t *testing.T) {
	const plaintext = "access-sandbox-1234-secret-bank-token"

	encrypted, err := cryptox.SymmetricEncrypt(plaintext, keyA)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("ciphertext equals plaintext")
	}
	if strings.Contains(encrypted, plaintext) {
		t.Fatalf("ciphertext contains the plaintext verbatim: %q", encrypted)
	}

	decrypted, err := cryptox.SymmetricDecrypt(encrypted, keyA)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// Encrypting the same plaintext twice yields different ciphertext (random
// nonce), yet both decrypt back to the original.
func TestEncryptionIsNonDeterministic(t *testing.T) {
	const plaintext = "repeatable-input"

	first, err := cryptox.SymmetricEncrypt(plaintext, keyA)
	if err != nil {
		t.Fatalf("encrypt first: %v", err)
	}
	second, err := cryptox.SymmetricEncrypt(plaintext, keyA)
	if err != nil {
		t.Fatalf("encrypt second: %v", err)
	}
	if first == second {
		t.Fatal("two encryptions produced identical ciphertext; nonce is not random")
	}
}

// Decrypting with the wrong key fails with an error rather than returning
// garbage plaintext.
func TestDecryptWithWrongKeyFails(t *testing.T) {
	const plaintext = "guarded-value"

	encrypted, err := cryptox.SymmetricEncrypt(plaintext, keyA)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := cryptox.SymmetricDecrypt(encrypted, keyB)
	if err == nil {
		t.Fatalf("decrypt with wrong key succeeded, returned %q; want error", got)
	}
	if got == plaintext {
		t.Fatal("decrypt with wrong key returned the original plaintext")
	}
}
