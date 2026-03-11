package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
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
		if isFTPNotFoundError(err) {
			return fmt.Errorf("failed to download file: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}

func (f *ftpStorage) UploadBytes(ctx context.Context, relPath string, data []byte) error {
	return f.Upload(ctx, relPath, bytes.NewReader(data))
}

func (f *ftpStorage) DownloadBytes(ctx context.Context, filename string) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Download(ctx, filename, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (f *ftpStorage) Delete(ctx context.Context, filename string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	cleanRel := strings.TrimPrefix(path.Clean("/"+filename), "/")
	remotePath := path.Join(f.basePath, cleanRel)
	if err := f.client.Delete(remotePath); err != nil {
		if isFTPNotFoundError(err) {
			return fmt.Errorf("failed to delete file: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

func (f *ftpStorage) List(ctx context.Context) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	return f.listRemoteFiles(f.basePath)
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

func isFTPNotFoundError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "550") || strings.Contains(msg, "not found")
}
