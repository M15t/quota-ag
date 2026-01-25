package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
)

// HashEmail creates a SHA256 hash of the email for use as filename
func HashEmail(email string) string {
	hash := sha256.Sum256([]byte(email))
	// Use first 16 bytes (32 hex chars) for reasonable filename length
	return hex.EncodeToString(hash[:16])
}

// Cached machine key to avoid repeated syscalls and hashing
var (
	cachedMachineKey    []byte
	cachedMachineKeyErr error
	machineKeyOnce      sync.Once
)

// GetMachineKey derives an encryption key from machine-specific identifiers
// The key is cached after first computation for performance.
func GetMachineKey() ([]byte, error) {
	machineKeyOnce.Do(func() {
		cachedMachineKey, cachedMachineKeyErr = deriveMachineKey()
	})
	return cachedMachineKey, cachedMachineKeyErr
}

// deriveMachineKey performs the actual key derivation
func deriveMachineKey() ([]byte, error) {
	// Combine multiple machine identifiers for key derivation
	var builder strings.Builder

	// Get hostname
	hostname, err := os.Hostname()
	if err == nil {
		builder.WriteString(hostname)
	}

	// Get current user
	currentUser, err := user.Current()
	if err == nil {
		builder.WriteString(currentUser.Username)
		builder.WriteString(currentUser.Uid)
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		builder.WriteString(homeDir)
	}

	// Add OS and arch for extra uniqueness
	builder.WriteString(runtime.GOOS)
	builder.WriteString(runtime.GOARCH)

	// Add a salt specific to this application
	builder.WriteString("quota-ag-v1-salt")

	machineID := builder.String()
	if machineID == "" {
		return nil, fmt.Errorf("failed to derive machine key: no identifiers available")
	}

	// Hash to get a 32-byte key for AES-256
	key := sha256.Sum256([]byte(machineID))
	return key[:], nil
}

// Encrypt encrypts plaintext using AES-256-GCM
func Encrypt(plaintext []byte) ([]byte, error) {
	key, err := GetMachineKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Create a nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func Decrypt(ciphertext []byte) ([]byte, error) {
	key, err := GetMachineKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
