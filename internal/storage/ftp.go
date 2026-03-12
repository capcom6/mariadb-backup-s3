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

// validateFilename checks if the filename is valid (not empty, not absolute, no path traversal).
// It returns a cleaned relative path or an error if the path is invalid or escapes basePath.
func (f *ftpStorage) validateFilename(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("%w: filename cannot be empty", ErrInvalidArgument)
	}

	// Reject absolute paths
	if path.IsAbs(filename) {
		return "", fmt.Errorf("%w: filename cannot be absolute", ErrInvalidArgument)
	}

	// Clean the path and ensure it's relative
	cleanRel := strings.TrimPrefix(path.Clean("/"+filename), "/")

	// Check for path traversal attempts - if after cleaning the path still contains "..",
	// it indicates an escape attempt
	if strings.HasPrefix(cleanRel, "..") || strings.Contains(cleanRel, "/../") {
		return "", fmt.Errorf("%w: filename contains path traversal", ErrInvalidArgument)
	}

	// Compute the full remote path
	remotePath := path.Join(f.basePath, cleanRel)

	// Verify the resulting path is still within basePath by checking prefix
	// We need to ensure basePath ends with "/" for proper prefix checking
	basePrefix := f.basePath
	if !strings.HasSuffix(basePrefix, "/") {
		basePrefix += "/"
	}
	if !strings.HasPrefix(remotePath+"/", basePrefix) && remotePath != f.basePath {
		return "", fmt.Errorf("%w: filename escapes base path", ErrInvalidArgument)
	}

	return cleanRel, nil
}

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

func (f *ftpStorage) Upload(ctx context.Context, filename string, data io.Reader) error {
	// Validate filename
	cleanRel, err := f.validateFilename(filename)
	if err != nil {
		return err
	}
	remotePath := path.Join(f.basePath, cleanRel)

	// Ensure remote directory exists
	if dirErr := f.ensureRemoteDir(path.Dir(remotePath)); dirErr != nil {
		return fmt.Errorf("failed to ensure remote directory: %w", dirErr)
	}

	// Fail fast if already canceled
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	// Upload file
	tmpPath := remotePath + ".tmp"
	if stErr := f.client.Store(tmpPath, data); stErr != nil {
		return fmt.Errorf("failed to upload file: %w", stErr)
	}

	if rnErr := f.client.Rename(tmpPath, remotePath); rnErr != nil {
		if delErr := f.client.Delete(tmpPath); delErr != nil {
			return fmt.Errorf("failed to rename file and delete temporary file: %w", errors.Join(rnErr, delErr))
		}
		return fmt.Errorf("failed to rename file: %w", rnErr)
	}

	return nil
}

func (f *ftpStorage) Download(ctx context.Context, filename string, data io.Writer) error {
	// Validate filename
	cleanRel, err := f.validateFilename(filename)
	if err != nil {
		return err
	}
	remotePath := path.Join(f.basePath, cleanRel)

	// Fail fast if already canceled
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	// Download file
	err = f.client.Retrieve(remotePath, data)
	if err != nil {
		if isFTPNotFoundError(err) {
			return fmt.Errorf("failed to download file: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}

func (f *ftpStorage) UploadBytes(ctx context.Context, filename string, data []byte) error {
	return f.Upload(ctx, filename, bytes.NewReader(data))
}

func (f *ftpStorage) DownloadBytes(ctx context.Context, filename string) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Download(ctx, filename, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (f *ftpStorage) Delete(ctx context.Context, filename string) error {
	// Validate filename
	cleanRel, err := f.validateFilename(filename)
	if err != nil {
		return err
	}
	remotePath := path.Join(f.basePath, cleanRel)

	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
	}

	if delErr := f.client.Delete(remotePath); delErr != nil {
		if isFTPNotFoundError(delErr) {
			return fmt.Errorf("failed to delete file: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete file: %w", delErr)
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

// isFTPNotFoundError checks if the error is a true "file not found" error.
// It inspects both the FTP error code and the message content to distinguish
// between file not found errors and permission/authentication errors.
// Only returns true when the error explicitly indicates a missing file
// (e.g., contains phrases like "No such file", "file not found", "not found").
func isFTPNotFoundError(err error) bool {
	var ftpErr goftp.Error

	if !errors.As(err, &ftpErr) || ftpErr.Code() != 550 {
		return false
	}

	msg := strings.ToLower(ftpErr.Message())

	// Check for clear "file not found" indicators
	notFoundIndicators := []string{
		"no such file",
		"file not found",
		"not found",
		"does not exist",
		"cannot find",
	}

	for _, indicator := range notFoundIndicators {
		if strings.Contains(msg, indicator) {
			return true
		}
	}

	return false
}
