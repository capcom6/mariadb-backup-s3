package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type fileLock struct {
	file *os.File
}

func (l *fileLock) Unlock(_ context.Context) error {
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}
	if err := os.Remove(l.file.Name()); err != nil {
		return fmt.Errorf("failed to unlock: %w", err)
	}

	return nil
}

type filesystemStorage struct {
	basePath string
}

func NewFilesystemStorage(u *url.URL) (Backend, error) {
	basePath := u.Path
	if basePath == "" {
		return nil, fmt.Errorf("%w: base path is required", ErrInvalidArgument)
	}

	return &filesystemStorage{
		basePath: basePath,
	}, nil
}

func (f *filesystemStorage) Upload(ctx context.Context, filename string, data io.Reader) (err error) {
	// Ensure the directory exists
	if err = os.MkdirAll(f.basePath, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath, err := f.makePath(filename)
	if err != nil {
		return err
	}

	file, err := os.CreateTemp(f.basePath, filepath.Base(filename))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close file: %w", closeErr))
		}
		if err != nil {
			if rmErr := os.Remove(file.Name()); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to remove partial file: %w", rmErr))
			}
		}
	}()

	// Use context-aware copying for cancellation support
	if err = copyWithContext(ctx, file, data); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if err = os.Rename(file.Name(), fullPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

func (f *filesystemStorage) Download(ctx context.Context, path string, data io.Writer) (err error) {
	fullPath, err := f.makePath(path)
	if err != nil {
		return err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to open file: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to open file: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close file: %w", closeErr))
		}
	}()

	// Use context-aware copying for cancellation support
	if cpyErr := copyWithContext(ctx, data, file); cpyErr != nil {
		err = fmt.Errorf("failed to read file: %w", cpyErr)
	}

	return err
}

func (f *filesystemStorage) UploadBytes(ctx context.Context, filename string, data []byte) error {
	return f.Upload(ctx, filename, bytes.NewReader(data))
}

func (f *filesystemStorage) DownloadBytes(ctx context.Context, filename string) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Download(ctx, filename, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (f *filesystemStorage) Delete(ctx context.Context, filename string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("filesystem: %w", ctx.Err())
	default:
	}

	fullPath, err := f.makePath(filename)
	if err != nil {
		return err
	}

	if rmErr := os.Remove(fullPath); rmErr != nil {
		if os.IsNotExist(rmErr) {
			return fmt.Errorf("failed to delete file: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete file: %w", rmErr)
	}

	return nil
}

func (f *filesystemStorage) List(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(f.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("filesystem: %w", ctx.Err())
		default:
		}

		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}

	return files, nil
}

// Lock implements [Backend].
func (f *filesystemStorage) Lock(_ context.Context, filename string) (Locker, error) {
	fullname, err := f.makePath(filename)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(fullname+".lock", os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: lock already held: %w", ErrLockFailed, err)
		}
		return nil, fmt.Errorf("%w: failed to create lock file: %w", ErrLockFailed, err)
	}

	return &fileLock{file: file}, nil
}

func (f *filesystemStorage) makePath(filename string) (string, error) {
	cleanRel := strings.TrimPrefix(filepath.Clean(string(filepath.Separator)+filename), string(filepath.Separator))
	fullPath := filepath.Join(f.basePath, cleanRel)
	rel, err := filepath.Rel(f.basePath, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: invalid filename", ErrInvalidArgument)
	}
	return fullPath, nil
}

// copyWithContext performs io.Copy with context cancellation support.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	// Create a buffer for efficient copying
	const bufferSize = 64 * 1024
	buf := make([]byte, bufferSize) // 64KB buffer

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("copy: %w", ctx.Err())
		default:
			n, err := src.Read(buf)
			if n > 0 {
				if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
					return fmt.Errorf("write error: %w", writeErr)
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("read error: %w", err)
			}
		}
	}
}

// Close closes the filesystem storage backend.
// For filesystem storage, this is a no-op as no persistent connections are held.
func (f *filesystemStorage) Close() error {
	return nil
}
