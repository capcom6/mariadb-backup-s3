package storage

import (
	"fmt"
	"net/url"
)

// New creates a new storage backend based on the configuration
func New(u *url.URL) (StorageBackend, error) {
	switch u.Scheme {
	case "s3":
		return NewS3Storage(u)
	case "file":
		return NewFilesystemStorage(u)
	case "gcs":
		return NewGCSStorage(u)
	case "azure":
		return NewAzureStorage(u)
	case "ftp":
		return NewFTPStorage(u)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", u.Scheme)
	}
}
