package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/secsy/goftp"
)

var (
	ErrFTPHostRequired = errors.New("FTP storage requires host")
)

type ftpStorage struct {
	host     string
	username string
	password string
	basePath string
	client   *goftp.Client
}

func NewFTPStorage(u *url.URL) (Backend, error) {
	// Extract credentials from URL
	var username, password string
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}
	if username == "" {
		username = "anonymous"
	}

	// Extract host and port
	host := u.Hostname()
	if host == "" {
		return nil, ErrFTPHostRequired
	}

	port := u.Port()
	if port == "" {
		port = "21" // Default FTP port
	}

	// Extract path
	basePath := u.Path
	if basePath == "" {
		basePath = "/"
	}
	basePath = path.Clean("/" + strings.TrimSuffix(basePath, "/"))

	// Create FTP client
	config := goftp.Config{
		User:     username,
		Password: password,
	}

	addr := net.JoinHostPort(host, port)
	client, err := goftp.DialConfig(config, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to FTP server: %w", err)
	}

	return &ftpStorage{
		host:     host,
		username: username,
		password: password,
		basePath: basePath,
		client:   client,
	}, nil
}

func (f *ftpStorage) Upload(ctx context.Context, relPath string, data io.Reader) error {
	// Create full remote path
	remotePath := path.Join(f.basePath, relPath)

	// Ensure remote directory exists
	if err := f.ensureRemoteDir(path.Dir(remotePath)); err != nil {
		return fmt.Errorf("failed to ensure remote directory: %w", err)
	}

	// Fail fast if already canceled
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	// Upload file
	tmpPath := remotePath + ".tmp"
	if err := f.client.Store(tmpPath, data); err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	if err := f.client.Rename(tmpPath, remotePath); err != nil {
		if delErr := f.client.Delete(tmpPath); delErr != nil {
			return fmt.Errorf("failed to rename file and delete temporary file: %w", errors.Join(err, delErr))
		}
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

func (f *ftpStorage) Download(ctx context.Context, filename string, data io.Writer) error {
	// Create full remote path (prevent escaping basePath)
	cleanRel := strings.TrimPrefix(path.Clean("/"+filename), "/")
	remotePath := path.Join(f.basePath, cleanRel)

	// Fail fast if already canceled
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	// Download file
	err := f.client.Retrieve(remotePath, data)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}

func (f *ftpStorage) DeleteOldBackups(ctx context.Context, maxCount int) error {
	if maxCount == 0 {
		return nil
	}

	// List files in remote directory
	files, err := f.listRemoteFiles(f.basePath)
	if err != nil {
		return fmt.Errorf("failed to remove old backups: %w", err)
	}

	if len(files) <= maxCount {
		return nil
	}

	// Sort files by name (which includes timestamp) to delete oldest
	sort.Strings(files)

	// Delete oldest files
	errs := make([]error, 0)
	toDelete := files[:len(files)-maxCount]
	for _, file := range toDelete {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		default:
		}

		remotePath := path.Join(f.basePath, file)
		if delErr := f.client.Delete(remotePath); delErr != nil {
			errs = append(errs, fmt.Errorf("failed to delete file %s: %w", file, delErr))
		}
	}

	return errors.Join(errs...)
}

// ensureRemoteDir ensures that a remote directory exists, creating it if necessary.
func (f *ftpStorage) ensureRemoteDir(dir string) error {
	if dir == "" || dir == "/" {
		return nil
	}

	// Check if directory exists
	if _, err := f.client.Stat(dir); err == nil {
		return nil // Directory exists
	}

	// Create parent directory first
	parent := path.Dir(dir)
	if parent != "/" && parent != dir {
		if err := f.ensureRemoteDir(parent); err != nil {
			return err
		}
	}

	// Create directory
	if _, err := f.client.Mkdir(dir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// listRemoteFiles lists files in a remote directory.
func (f *ftpStorage) listRemoteFiles(dir string) ([]string, error) {
	files, err := f.client.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var fileNames []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileNames = append(fileNames, file.Name())
	}

	return fileNames, nil
}

// Close closes the FTP connection.
func (f *ftpStorage) Close() error {
	if f.client != nil {
		if err := f.client.Close(); err != nil {
			return fmt.Errorf("failed to close FTP connection: %w", err)
		}
		f.client = nil
	}
	return nil
}
