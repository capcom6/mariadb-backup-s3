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

	// Ensure the directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	return &filesystemStorage{
		basePath: basePath,
	}, nil
}

func (f *filesystemStorage) Upload(ctx context.Context, path string, data io.Reader) (err error) {
	fullPath := filepath.Join(f.basePath, path)

	// Ensure the directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Use context-aware copying for cancellation support
	if err := copyWithContext(ctx, file, data); err != nil {
		// Clean up the file on error
		_ = os.Remove(fullPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
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
