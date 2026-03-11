package storage

import (
	"context"
	"io"
)

// Backend defines the interface for pluggable storage backends.
type Backend interface {
	// Upload uploads data to the specified path in the storage backend
	Upload(ctx context.Context, filename string, data io.Reader) error

	// Download downloads data from the specified path in the storage backend
	Download(ctx context.Context, filename string, data io.Writer) error

	// Delete removes a single object from the storage backend.
	Delete(ctx context.Context, filename string) error

	// List returns filenames available in the current backend prefix/path.
	List(ctx context.Context) ([]string, error)

	// UploadBytes uploads a small in-memory object to the backend.
	UploadBytes(ctx context.Context, filename string, data []byte) error

	// DownloadBytes downloads a small object from the backend into memory.
	DownloadBytes(ctx context.Context, filename string) ([]byte, error)
}
