package encryption_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAES256GCMService_EncryptDecryptStreamRoundtrip(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "Empty data",
			data: []byte(""),
		},
		{
			name: "Short data",
			data: []byte("test stream encryption"),
		},
		{
			name: "Medium data",
			data: []byte("This is a test message for streaming encryption and decryption"),
		},
		{
			name: "Long data",
			data: make([]byte, 1024*10), // 10KB
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test streaming encryption
			input := bytes.NewReader(tc.data)
			var encrypted bytes.Buffer

			err := service.Encrypt(context.Background(), input, &encrypted)
			assert.NoError(t, err, "Streaming encryption failed")
			assert.Greater(t, encrypted.Len(), len(tc.data), "Encrypted data should be larger than original")

			// Test streaming decryption
			encryptedReader := bytes.NewReader(encrypted.Bytes())
			var decrypted bytes.Buffer

			err = service.Decrypt(context.Background(), encryptedReader, &decrypted)
			assert.NoError(t, err, "Streaming decryption failed")

			// Handle empty byte slice comparison
			if len(tc.data) == 0 {
				assert.Equal(t, 0, decrypted.Len(), "Decrypted empty data should be empty")
			} else {
				assert.Equal(t, tc.data, decrypted.Bytes(), "Decrypted data should match original")
			}
		})
	}
}

func TestAES256GCMService_EncryptDecryptStreamLargeData(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Generate 1MB+ test data
	testData := make([]byte, 1024*1024*2) // 2MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Test streaming encryption/decryption of large data
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer

	err := service.Encrypt(context.Background(), input, &encrypted)
	assert.NoError(t, err, "Large data streaming encryption failed")
	assert.Greater(t, encrypted.Len(), len(testData), "Encrypted data should be larger than original")

	// Test streaming decryption
	encryptedReader := bytes.NewReader(encrypted.Bytes())
	var decrypted bytes.Buffer

	err = service.Decrypt(context.Background(), encryptedReader, &decrypted)
	assert.NoError(t, err, "Large data streaming decryption failed")
	assert.Equal(t, testData, decrypted.Bytes(), "Decrypted large data should match original")
}

func TestAES256GCMService_DecryptStreamTruncatedData(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data
	testData := []byte("This is test data for truncation testing")

	// Encrypt the data first
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for truncation test failed")

	// Test with truncated encrypted data (missing nonce)
	truncatedData := encrypted.Bytes()[:5] // Too short, missing nonce
	truncatedReader := bytes.NewReader(truncatedData)
	var output bytes.Buffer

	err = service.Decrypt(context.Background(), truncatedReader, &output)
	assert.Error(t, err, "Decryption should fail with truncated data")
}

func TestAES256GCMService_DecryptStreamTamperedData(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data
	testData := []byte("This is test data for tampering testing")

	// Encrypt the data first
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for tampering test failed")

	// Tamper with encrypted data (flip some bits in the ciphertext part)
	encryptedBytes := encrypted.Bytes()
	if len(encryptedBytes) > 20 {
		encryptedBytes[20] ^= 0xFF // Flip some bits in the ciphertext
	}

	// Test decryption should fail with authentication error
	tamperedReader := bytes.NewReader(encryptedBytes)
	var output bytes.Buffer

	err = service.Decrypt(context.Background(), tamperedReader, &output)
	assert.Error(t, err, "Decryption should fail with tampered data")
	assert.Contains(t, err.Error(), "message authentication failed", "Error should indicate tag mismatch")
}

func BenchmarkAES256GCMService_EncryptStream(b *testing.B) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data (1MB)
	testData := make([]byte, 1024*1024)
	input := bytes.NewReader(testData)

	for b.Loop() {
		var output bytes.Buffer
		err := service.Encrypt(context.Background(), input, &output)
		if err != nil {
			b.Error(err)
		}
		// Reset input for next iteration
		input = bytes.NewReader(testData)
	}
}

func BenchmarkAES256GCMService_DecryptStream(b *testing.B) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data (1MB)
	testData := make([]byte, 1024*1024)
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer

	// Encrypt once to get test data
	err := service.Encrypt(context.Background(), input, &encrypted)
	if err != nil {
		b.Error(err)
		return
	}

	encryptedReader := bytes.NewReader(encrypted.Bytes())

	for b.Loop() {
		var output bytes.Buffer
		err := service.Decrypt(context.Background(), encryptedReader, &output)
		if err != nil {
			b.Error(err)
		}
		// Reset reader for next iteration
		encryptedReader = bytes.NewReader(encrypted.Bytes())
	}
}
