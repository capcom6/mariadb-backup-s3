package storage

import (
	"context"
	"io"
)

// StorageBackend defines the interface for pluggable storage backends
type StorageBackend interface {
	// Upload uploads data to the specified path in the storage backend
	Upload(ctx context.Context, path string, data io.Reader) error

	// DeleteOldBackups removes old backups based on the retention policy
	DeleteOldBackups(ctx context.Context, maxCount int) error
}
