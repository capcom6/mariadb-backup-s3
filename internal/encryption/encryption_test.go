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

func TestAES256GCMService_DecryptStreamContextCancellation(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data - use larger data to ensure we hit the context check
	testData := make([]byte, 1024*100) // 100KB to ensure multiple chunks
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Encrypt the data first
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for context test failed")

	// Test decryption with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	encryptedReader := bytes.NewReader(encrypted.Bytes())
	var output bytes.Buffer

	err = service.Decrypt(ctx, encryptedReader, &output)
	assert.Error(t, err, "Decryption should fail with cancelled context")
	if err != nil {
		assert.Contains(t, err.Error(), "context canceled", "Error should indicate context cancellation")
	}
}

func TestAES256GCMService_DecryptStreamInvalidMagic(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Create data with invalid magic header
	invalidData := []byte("INVALIDMAGIC")
	invalidReader := bytes.NewReader(invalidData)
	var output bytes.Buffer

	err := service.Decrypt(context.Background(), invalidReader, &output)
	assert.Error(t, err, "Decryption should fail with invalid magic")
	assert.Contains(t, err.Error(), "bad magic", "Error should indicate bad magic")
}

func TestAES256GCMService_DecryptStreamInvalidVersion(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Create valid encrypted data first
	testData := []byte("This is test data for version testing")
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for version test failed")

	// Modify version byte to invalid value
	encryptedBytes := encrypted.Bytes()
	if len(encryptedBytes) > 5 {
		encryptedBytes[4] = 0xFF // Invalid version
	}

	invalidReader := bytes.NewReader(encryptedBytes)
	var output bytes.Buffer

	err = service.Decrypt(context.Background(), invalidReader, &output)
	assert.Error(t, err, "Decryption should fail with invalid version")
	assert.Contains(t, err.Error(), "unsupported version", "Error should indicate unsupported version")
}

func TestAES256GCMService_DecryptStreamWrongKey(t *testing.T) {
	// Setup
	correctKey := make([]byte, 32)
	for i := range correctKey {
		correctKey[i] = byte(i)
	}
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(0xFF - i) // Different key
	}

	// Encrypt with correct key
	service := encryption.NewAES256GCMService(correctKey)
	testData := []byte("This is test data for wrong key testing")
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for wrong key test failed")

	// Try to decrypt with wrong key
	wrongService := encryption.NewAES256GCMService(wrongKey)
	encryptedReader := bytes.NewReader(encrypted.Bytes())
	var output bytes.Buffer

	err = wrongService.Decrypt(context.Background(), encryptedReader, &output)
	assert.Error(t, err, "Decryption should fail with wrong key")
	// AES-GCM will fail with authentication error when key is wrong
}

func TestAES256GCMService_DecryptStreamPartialChunk(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data that will create partial chunks
	testData := make([]byte, 1024*64+100) // chunkSize + 100 bytes
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Encrypt the data first
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for partial chunk test failed")

	// Test decryption
	encryptedReader := bytes.NewReader(encrypted.Bytes())
	var decrypted bytes.Buffer

	err = service.Decrypt(context.Background(), encryptedReader, &decrypted)
	assert.NoError(t, err, "Decryption should handle partial chunks correctly")
	assert.Equal(t, testData, decrypted.Bytes(), "Decrypted data should match original with partial chunks")
}

func TestAES256GCMService_DecryptStreamMultipleChunks(t *testing.T) {
	// Setup
	encryptionKey := make([]byte, 32)
	service := encryption.NewAES256GCMService(encryptionKey)

	// Test data that spans multiple chunks exactly
	chunkSize := 64 * 1024                // 64 KiB chunks
	testData := make([]byte, chunkSize*3) // Exactly 3 chunks
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Encrypt the data first
	input := bytes.NewReader(testData)
	var encrypted bytes.Buffer
	err := service.Encrypt(context.Background(), input, &encrypted)
	require.NoError(t, err, "Encryption for multiple chunks test failed")

	// Test decryption
	encryptedReader := bytes.NewReader(encrypted.Bytes())
	var decrypted bytes.Buffer

	err = service.Decrypt(context.Background(), encryptedReader, &decrypted)
	assert.NoError(t, err, "Decryption should handle multiple chunks correctly")
	assert.Equal(t, testData, decrypted.Bytes(), "Decrypted data should match original with multiple chunks")
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
