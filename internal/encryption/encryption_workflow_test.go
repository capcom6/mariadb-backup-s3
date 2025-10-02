package encryption_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/stretchr/testify/assert"
)

func TestAES256GCMStreamingWorkflow(t *testing.T) {
	// Generate a random 32-byte key for testing
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i) // Use a predictable pattern for testing
	}

	// Encode key as base64 for environment variable
	encryptionKey := key

	// Create encryption service
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data - use larger data to test streaming behavior
	testData := []byte("This is a test message for streaming encryption workflow verification with sufficient data to trigger chunked processing")

	// Test streaming encryption
	ctx := context.Background()
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer

	err := service.Encrypt(ctx, input, &encrypted)
	assert.NoError(t, err, "Streaming encryption failed")
	assert.NotNil(t, encrypted, "Encrypted data should not be nil")
	assert.Greater(t, encrypted.Len(), len(testData), "Encrypted data should be larger than original due to nonce and auth tag")

	// Create a new service instance for decryption (since keys are zeroized)
	decryptService := encryption.NewAES256GCMService(encryptionKey)

	// Test streaming decryption
	encryptedReader := bytes.NewReader(encrypted.Bytes())
	var decrypted bytes.Buffer

	err = decryptService.Decrypt(ctx, encryptedReader, &decrypted)
	assert.NoError(t, err, "Streaming decryption failed")
	assert.Equal(t, testData, decrypted.Bytes(), "Decrypted data should match original")
}

func TestAES256GCMStreamingWorkflowTamperedData(t *testing.T) {
	// Generate a random 32-byte key for testing
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// Encode key as base64 for environment variable
	encryptionKey := key

	// Create encryption service
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data
	testData := []byte("This is a test message for streaming tampering verification")

	// Test streaming encryption
	ctx := context.Background()
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer

	err := service.Encrypt(ctx, input, &encrypted)
	assert.NoError(t, err, "Streaming encryption for tampering test failed")

	// Tamper with encrypted data (flip some bits in the ciphertext part)
	encryptedBytes := encrypted.Bytes()
	if len(encryptedBytes) > 25 {
		encryptedBytes[25] ^= 0xFF // Flip some bits in the ciphertext
	}

	// Create a new service instance for decryption (since keys are zeroized)
	decryptService := encryption.NewAES256GCMService(encryptionKey)

	// Test streaming decryption should fail with authentication error
	tamperedReader := bytes.NewReader(encryptedBytes)
	var output bytes.Buffer

	err = decryptService.Decrypt(ctx, tamperedReader, &output)
	assert.Error(t, err, "Streaming decryption should fail with tampered data")
}
