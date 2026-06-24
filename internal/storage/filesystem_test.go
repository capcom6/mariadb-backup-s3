package storage_test

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

func newFilesystem(t *testing.T) (storage.Backend, string) {
	t.Helper()
	dir := t.TempDir()
	u := &url.URL{Path: dir}
	s, err := storage.NewFilesystemStorage(u)
	require.NoError(t, err)
	return s, dir
}

func TestNewFilesystemStorage_EmptyPath(t *testing.T) {
	u := &url.URL{Path: ""}
	_, err := storage.NewFilesystemStorage(u)
	assert.ErrorIs(t, err, storage.ErrInvalidArgument)
}

func TestFilesystemUpload_Download_Roundtrip(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	data := []byte("hello, world!")
	err := s.Upload(ctx, "test.txt", bytes.NewReader(data))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = s.Download(ctx, "test.txt", &buf)
	require.NoError(t, err)
	assert.Equal(t, data, buf.Bytes())
}

func TestFilesystemUpload_CreatesBaseDir(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "new", "nested", "dir")
	u := &url.URL{Path: baseDir}
	s, err := storage.NewFilesystemStorage(u)
	require.NoError(t, err)
	ctx := context.Background()

	err = s.Upload(ctx, "file.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	_, err = os.Stat(baseDir)
	assert.NoError(t, err)
}

func TestFilesystemUpload_NestedPathFails(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "sub/dir/file.txt", bytes.NewReader([]byte("data")))
	assert.Error(t, err)
}

func TestFilesystemUpload_ContextCancellation(t *testing.T) {
	s, dir := newFilesystem(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Upload(ctx, "canceled.txt", bytes.NewReader([]byte("data")))
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFilesystemUpload_ContextCancellation_MidUpload(t *testing.T) {
	s, dir := newFilesystem(t)

	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Upload(ctx, "large.bin", pr)
	}()

	_, err := pw.Write(make([]byte, 128*1024))
	require.NoError(t, err)
	cancel()
	pw.Close()

	err = <-errCh
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFilesystemUpload_ContextCancellation_PartialWrite(t *testing.T) {
	s, dir := newFilesystem(t)

	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Upload(ctx, "partial.bin", pr)
	}()

	_, err := pw.Write([]byte("some data"))
	require.NoError(t, err)
	cancel()
	pw.Close()

	err = <-errCh
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFilesystemDownload_NotFound(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	var buf bytes.Buffer
	err := s.Download(ctx, "nonexistent.txt", &buf)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestFilesystemDelete_Success(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "todelete.txt", bytes.NewReader([]byte("delete me")))
	require.NoError(t, err)

	err = s.Delete(ctx, "todelete.txt")
	assert.NoError(t, err)
}

func TestFilesystemDelete_NotFound(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Delete(ctx, "nonexistent.txt")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestFilesystemDelete_ContextCancellation(t *testing.T) {
	s, _ := newFilesystem(t)

	err := s.Upload(context.Background(), "file.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = s.Delete(ctx, "file.txt")
	assert.Error(t, err)
}

func TestFilesystemList_ReturnsFiles(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		err := s.Upload(ctx, name, bytes.NewReader([]byte("data")))
		require.NoError(t, err)
	}

	files, err := s.List(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.txt", "b.txt", "c.txt"}, files)
}

func TestFilesystemList_EmptyDir(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	files, err := s.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestFilesystemList_NonExistentDir(t *testing.T) {
	u := &url.URL{Path: filepath.Join(t.TempDir(), "nonexistent")}
	s, err := storage.NewFilesystemStorage(u)
	require.NoError(t, err)
	ctx := context.Background()

	files, err := s.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestFilesystemList_SkipsDirectories(t *testing.T) {
	s, dir := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "file.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(dir, "subdir"), 0700)
	require.NoError(t, err)

	files, err := s.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"file.txt"}, files)
}

func TestFilesystemList_ContextCancellation(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx, cancel := context.WithCancel(context.Background())

	err := s.Upload(ctx, "file.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	cancel()

	_, err = s.List(ctx)
	assert.Error(t, err)
}

func TestFilesystemLock_AcquireUnlock(t *testing.T) {
	s, dir := newFilesystem(t)
	ctx := context.Background()

	locker, err := s.Lock(ctx, "test.txt")
	require.NoError(t, err)
	require.NotNil(t, locker)

	_, err = os.Stat(filepath.Join(dir, "test.txt.lock"))
	require.NoError(t, err)

	err = locker.Unlock(ctx)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "test.txt.lock"))
	assert.True(t, os.IsNotExist(err))
}

func TestFilesystemLock_DoubleLock(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	locker, err := s.Lock(ctx, "test.txt")
	require.NoError(t, err)
	defer func() { _ = locker.Unlock(ctx) }()

	_, err = s.Lock(ctx, "test.txt")
	assert.ErrorIs(t, err, storage.ErrLockFailed)
}

func TestFilesystemMakePath_NormalizesRelativeTraversal(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "target.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	// Download using ../target.txt should resolve to <basePath>/target.txt
	var buf bytes.Buffer
	err = s.Download(ctx, "../target.txt", &buf)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), buf.Bytes())
}

func TestFilesystemMakePath_NormalizesDeepTraversal(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "target.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = s.Download(ctx, "../../../target.txt", &buf)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), buf.Bytes())
}

func TestFilesystemMakePath_NormalizesAbsolutePath(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "target.txt", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = s.Download(ctx, "/target.txt", &buf)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), buf.Bytes())
}

func TestFilesystemMakePath_EmptyFilename(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "", bytes.NewReader([]byte("data")))
	assert.Error(t, err)
}

func TestFilesystemMakePath_JustDot(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, ".", bytes.NewReader([]byte("data")))
	assert.Error(t, err)
}

func TestFilesystemUpload_LargeFile(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	err := s.Upload(ctx, "large.bin", bytes.NewReader(data))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = s.Download(ctx, "large.bin", &buf)
	require.NoError(t, err)
	assert.Equal(t, data, buf.Bytes())
}

func TestFilesystemUploadBytes_DownloadBytes(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	data := []byte("hello bytes")
	err := s.UploadBytes(ctx, "bytes.txt", data)
	require.NoError(t, err)

	got, err := s.DownloadBytes(ctx, "bytes.txt")
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestFilesystemUploadBytes_PathTraversal(t *testing.T) {
	s, dir := newFilesystem(t)
	ctx := context.Background()

	err := s.UploadBytes(ctx, "../outside.txt", []byte("data"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "outside.txt"))
	assert.NoError(t, err)
}

func TestFilesystemDownload_EmptyFile(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "empty.txt", bytes.NewReader([]byte{}))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = s.Download(ctx, "empty.txt", &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.Bytes())
}

func TestFilesystemUpload_Overwrite(t *testing.T) {
	s, _ := newFilesystem(t)
	ctx := context.Background()

	err := s.Upload(ctx, "file.txt", bytes.NewReader([]byte("first")))
	require.NoError(t, err)

	err = s.Upload(ctx, "file.txt", bytes.NewReader([]byte("second")))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = s.Download(ctx, "file.txt", &buf)
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), buf.Bytes())
}

func TestFilesystemClose(t *testing.T) {
	s, _ := newFilesystem(t)
	err := s.Close()
	assert.NoError(t, err)
}
