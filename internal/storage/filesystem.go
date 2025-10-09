package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
)

type filesystemStorage struct {
	basePath string
}

func NewFilesystemStorage(u *url.URL) (StorageBackend, error) {
	basePath := u.Path
	if basePath == "" {
		return nil, fmt.Errorf("filesystem storage requires a path")
	}

	return &filesystemStorage{
		basePath: basePath,
	}, nil
}

func (f *filesystemStorage) Upload(ctx context.Context, filename string, data io.Reader) error {
	var err error

	// Ensure the directory exists
	if err := os.MkdirAll(f.basePath, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath := filepath.Join(f.basePath, filename)
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = errors.Join(err, fmt.Errorf("failed to close file: %w", closeErr))
		}
	}()

	// Use context-aware copying for cancellation support
	if err := copyWithContext(ctx, file, data); err != nil {
		// Clean up the file on error
		_ = os.Remove(fullPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	return err
}

func (f *filesystemStorage) Download(ctx context.Context, path string, data io.Writer) error {
	var err error

	fullPath := filepath.Join(f.basePath, path)

	file, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = errors.Join(err, fmt.Errorf("failed to close file: %w", closeErr))
		}
	}()

	// Use context-aware copying for cancellation support
	if err := copyWithContext(ctx, data, file); err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return err
}

func (f *filesystemStorage) DeleteOldBackups(ctx context.Context, maxCount int) error {
	if maxCount == 0 {
		return nil
	}

	fullPath := f.basePath

	// Read directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, nothing to delete
		}
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Filter for files only
	files := make([]os.DirEntry, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}

	if len(files) <= maxCount {
		return nil
	}

	// Sort files by name (oldest first) - filenames contain sortable timestamps
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	// Delete oldest files
	toDelete := files[:len(files)-maxCount]
	for _, file := range toDelete {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		filePath := filepath.Join(fullPath, file.Name())
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to delete file %s: %w", file.Name(), err)
		}
	}

	return nil
}

// copyWithContext performs io.Copy with context cancellation support
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	// Create a buffer for efficient copying
	buf := make([]byte, 64*1024) // 64KB buffer

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := src.Read(buf)
			if n > 0 {
				if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
					return writeErr
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}
}
