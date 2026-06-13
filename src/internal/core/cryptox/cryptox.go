// Package cryptox provides symmetric encryption helpers used to protect secrets
// at rest (e.g. stored bank access tokens). The secret is a hex-encoded AES key;
// encryption uses AES-GCM with a random per-message nonce.
package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
)

func newCipher(secret string) (cipher.AEAD, error) {
	keyBytes, err := hex.DecodeString(secret)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// SymmetricEncrypt encrypts plaintext under secret and returns a base64-encoded
// nonce+ciphertext string.
func SymmetricEncrypt(plaintext, secret string) (string, error) {
	gcm, err := newCipher(secret)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// SymmetricDecrypt reverses SymmetricEncrypt. It returns an error when secret
// does not match the key the value was encrypted under, rather than garbage.
func SymmetricDecrypt(encoded, secret string) (string, error) {
	gcm, err := newCipher(secret)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	return string(plaintext), err
}
