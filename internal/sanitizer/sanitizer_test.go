package sanitizer_test

import (
	"testing"

	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
)

func TestSanitizeOptions(t *testing.T) {
	tests := []struct {
		name        string
		options     string
		expectError bool
	}{
		{
			name:        "Valid options",
			options:     "--parallel=4 --safe-option=value",
			expectError: false,
		},
		{
			name:        "Empty options",
			options:     "",
			expectError: false,
		},
		{
			name:        "Option with dangerous characters",
			options:     "--parallel=4; rm -rf /",
			expectError: true,
		},
		{
			name:        "Option with pipe",
			options:     "--parallel=4 | cat /etc/passwd",
			expectError: true,
		},
		{
			name:        "Option with dollar sign",
			options:     "--parallel=4 $(echo 'test')",
			expectError: true,
		},
		{
			name:        "Option with ampersand",
			options:     "--parallel=4 & sleep 10",
			expectError: true,
		},
		{
			name:        "Valid option with equals",
			options:     "--parallel=4 --target-dir=/backup",
			expectError: false,
		},
		{
			name:        "Invalid option format",
			options:     "parallel=4",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizer.SanitizeOptions(tt.options)
			if (err != nil) != tt.expectError {
				t.Errorf("SanitizeOptions() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}
