package storage

import (
	"context"
	"io"
)

// StorageBackend defines the interface for pluggable storage backends
type StorageBackend interface {
	// Upload uploads data to the specified path in the storage backend
	Upload(ctx context.Context, filename string, data io.Reader) error

	// Download downloads data from the specified path in the storage backend
	Download(ctx context.Context, filename string, data io.Writer) error

	// DeleteOldBackups removes old backups based on the retention policy
	DeleteOldBackups(ctx context.Context, maxCount int) error
}
