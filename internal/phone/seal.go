package phone

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// AAD is the additional data of each seal. It binds the ciphertext to this format.
const AAD = "i2a-push-v1"

// KeySize is the size of the pairing key in bytes (AES-256).
const KeySize = 32

const nonceSize = 12

// randReader is the source of nonces. Tests replace it to make fixed vectors.
var randReader io.Reader = rand.Reader

// Seal compresses plaintext with raw deflate and seals it with AES-256-GCM.
// The result is the standard base64 of nonce, ciphertext and tag.
func Seal(key, plaintext []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	var z bytes.Buffer
	w, err := flate.NewWriter(&z, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(plaintext); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, z.Bytes(), []byte(AAD))
	return base64.StdEncoding.EncodeToString(out), nil
}

// Open reverses Seal.
func Open(key []byte, sealed string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, fmt.Errorf("the sealed text is not base64: %w", err)
	}
	if len(raw) < nonceSize+gcm.Overhead() {
		return nil, errors.New("the sealed text is too short")
	}
	z, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], []byte(AAD))
	if err != nil {
		return nil, errors.New("the key does not open the message")
	}
	r := flate.NewReader(bytes.NewReader(z))
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 1<<20))
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("the key has %d bytes; it must have %d", len(key), KeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
