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
)

// HashEmail creates a SHA256 hash of the email for use as filename
func HashEmail(email string) string {
	hash := sha256.Sum256([]byte(email))
	// Use first 16 bytes (32 hex chars) for reasonable filename length
	return hex.EncodeToString(hash[:16])
}

// GetMachineKey derives an encryption key from machine-specific identifiers
func GetMachineKey() ([]byte, error) {
	// Combine multiple machine identifiers for key derivation
	var machineID string

	// Get hostname
	hostname, err := os.Hostname()
	if err == nil {
		machineID += hostname
	}

	// Get current user
	currentUser, err := user.Current()
	if err == nil {
		machineID += currentUser.Username + currentUser.Uid
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		machineID += homeDir
	}

	// Add OS and arch for extra uniqueness
	machineID += runtime.GOOS + runtime.GOARCH

	// Add a salt specific to this application
	machineID += "quota-ag-v1-salt"

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
