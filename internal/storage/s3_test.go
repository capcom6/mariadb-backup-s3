package storage_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

func TestNewS3Storage_PartSizeValidation_InvalidValues(t *testing.T) {
	// Test cases that should fail validation BEFORE AWS config is loaded
	tests := []struct {
		name        string
		partSize    string
		errContains string
	}{
		{
			name:        "invalid part size - below minimum",
			partSize:    "5242879",
			errContains: "part-size must be at least",
		},
		{
			name:        "invalid part size - above maximum",
			partSize:    "5368709121",
			errContains: "part-size must be at most",
		},
		{
			name:        "invalid part size - non-numeric",
			partSize:    "not-a-number",
			errContains: "failed to parse part-size",
		},
		{
			name:        "invalid part size - negative",
			partSize:    "-1",
			errContains: "part-size must be at least",
		},
		{
			name:        "invalid part size - zero",
			partSize:    "0",
			errContains: "part-size must be at least",
		},
		{
			name:        "invalid part size - decimal",
			partSize:    "10485760.5",
			errContains: "failed to parse part-size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse("s3://test-bucket/backups?part-size=" + tt.partSize)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			_, err = storage.NewS3Storage(u)

			if err == nil {
				t.Errorf("NewS3Storage() expected error containing %q, got nil", tt.errContains)
				return
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("NewS3Storage() error = %v, should contain %q", err, tt.errContains)
			}
		})
	}
}

func TestNewS3Storage_PartSize_ValidValues(t *testing.T) {
	// Test that valid part-size values don't fail at validation stage
	// These may succeed or fail later (e.g., at AWS config loading), but should NOT fail validation
	tests := []struct {
		name     string
		partSize string
	}{
		{
			name:     "default part size (not specified)",
			partSize: "",
		},
		{
			name:     "valid part size - minimum boundary",
			partSize: "5242880",
		},
		{
			name:     "valid part size - 20MB",
			partSize: "20971520",
		},
		{
			name:     "valid part size - 100MB",
			partSize: "104857600",
		},
		{
			name:     "valid part size - maximum boundary",
			partSize: "5368709120",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse("s3://test-bucket/backups")
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			q := u.Query()
			if tt.partSize != "" {
				q.Set("part-size", tt.partSize)
			}
			u.RawQuery = q.Encode()

			_, err = storage.NewS3Storage(u)

			// Should NOT get validation errors
			if err != nil {
				validationErrors := []string{
					"part-size must be at least",
					"part-size must be at most",
					"failed to parse part-size",
				}
				for _, ve := range validationErrors {
					if strings.Contains(err.Error(), ve) {
						t.Errorf("NewS3Storage() got validation error for valid value: %v", err)
						return
					}
				}
				// Other errors (e.g., AWS config) are acceptable for this test
				t.Logf("Got non-validation error (acceptable): %v", err)
			}
		})
	}
}

func TestNewS3Storage_PartSize_ErrInvalidArgument(t *testing.T) {
	// Test that validation errors wrap ErrInvalidArgument
	tests := []struct {
		name     string
		partSize string
	}{
		{
			name:     "below minimum",
			partSize: "1000",
		},
		{
			name:     "above maximum",
			partSize: "9999999999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse("s3://test-bucket/backups?part-size=" + tt.partSize)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			_, err = storage.NewS3Storage(u)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			if !errors.Is(err, storage.ErrInvalidArgument) {
				t.Errorf("Expected error to wrap ErrInvalidArgument, got: %v", err)
			}
		})
	}
}

func TestS3Storage_PartSize_Constants(t *testing.T) {
	// Verify constant values are correct
	if storage.S3DefaultPartSize != 10*1024*1024 {
		t.Errorf("S3DefaultPartSize = %d, want %d", storage.S3DefaultPartSize, 10*1024*1024)
	}

	if storage.S3MinPartSize != 5*1024*1024 {
		t.Errorf("S3MinPartSize = %d, want %d", storage.S3MinPartSize, 5*1024*1024)
	}

	if storage.S3MaxPartSize != 5*1024*1024*1024 {
		t.Errorf("S3MaxPartSize = %d, want %d", storage.S3MaxPartSize, 5*1024*1024*1024)
	}

	// Verify relationships
	if storage.S3MinPartSize >= storage.S3DefaultPartSize {
		t.Error("S3MinPartSize should be less than S3DefaultPartSize")
	}

	if storage.S3DefaultPartSize >= storage.S3MaxPartSize {
		t.Error("S3DefaultPartSize should be less than S3MaxPartSize")
	}
}
