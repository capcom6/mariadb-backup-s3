package storage

import (
	"errors"
	"fmt"
	"net/url"
)

var (
	ErrUnsupportedStorageType = errors.New("unsupported storage type")
)

// New creates a new storage backend based on the configuration.
func New(u *url.URL) (Backend, error) {
	switch u.Scheme {
	case "s3":
		return NewS3Storage(u)
	case "file":
		return NewFilesystemStorage(u)
	case "ftp":
		return NewFTPStorage(u)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedStorageType, u.Scheme)
	}
}
