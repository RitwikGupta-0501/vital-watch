package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

var (
	ErrDecryptionFailed = errors.New("failed to decrypt data: invalid key, corrupted ciphertext, or mismatched AAD")
	ErrMasterKeyMissing = errors.New("KMS_MASTER_KEY environment variable is not set")
)

// CipherService handles symmetric encryption for the application.
type CipherService struct {
	aead cipher.AEAD
}

// NewCipherService initializes a new CipherService.
// It derives a 32-byte AES-256 key from the provided master key string.
// In a production environment, this should ideally use a strong KDF (like Argon2 or PBKDF2)
// or integrate directly with a cloud KMS provider.
func NewCipherService(masterKeyStr string) (*CipherService, error) {
	if masterKeyStr == "" {
		return nil, ErrMasterKeyMissing
	}

	// Derive a 32-byte key from the master key string using SHA-256
	// Note: For enhanced security against brute-force on weak master keys,
	// consider migrating to PBKDF2 or Argon2.
	hash := sha256.Sum256([]byte(masterKeyStr))
	key := hash[:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM mode: %w", err)
	}

	return &CipherService{aead: aesgcm}, nil
}

// Encrypt encrypts a plaintext string using AES-256-GCM.
// The aad (Additional Authenticated Data) parameter binds the ciphertext to a specific context
// (e.g., a Tenant ID) to prevent confused deputy attacks.
func (s *CipherService) Encrypt(plaintext string, aad []byte) (ciphertext []byte, nonce []byte, err error) {
	nonce = make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext = s.aead.Seal(nil, nonce, []byte(plaintext), aad)
	return ciphertext, nonce, nil
}

// Decrypt decrypts an AES-256-GCM ciphertext.
// The aad must perfectly match the aad used during encryption.
func (s *CipherService) Decrypt(ciphertext []byte, nonce []byte, aad []byte) (plaintext string, err error) {
	ptBytes, err := s.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(ptBytes), nil
}
