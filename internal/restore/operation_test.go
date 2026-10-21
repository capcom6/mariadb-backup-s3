package restore_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/restore"
)

const streamedBackup = "-- mariadb-backup-s3: database=db1\ndump of db1\n" +
	"-- mariadb-backup-s3: database=db2\ndump of db2\n"

func TestOperation_Run_SQLStream(t *testing.T) {
	storageDir := t.TempDir()
	targetDir := t.TempDir()

	writeArtifact(t, storageDir, "2024-01-15-10-30-00.sql.gz", gzipBytes(t, []byte(streamedBackup)))

	cfg := restore.DefaultConfig()
	cfg.Storage.URL = "file://" + storageDir
	cfg.TargetDir = targetDir

	op := restore.NewOperation(cfg, logging.NewTestLogger())
	require.NoError(t, op.Run(context.Background(), "2024-01-15-10-30-00.sql.gz"))

	assertFileContent(t, filepath.Join(targetDir, "db1.sql"), "dump of db1\n")
	assertFileContent(t, filepath.Join(targetDir, "db2.sql"), "dump of db2\n")
}

func TestOperation_Run_SQLStream_Encrypted(t *testing.T) {
	storageDir := t.TempDir()
	targetDir := t.TempDir()

	key := []byte("0123456789abcdef0123456789abcdef")
	encrypted := encryptBytes(t, key, gzipBytes(t, []byte(streamedBackup)))
	writeArtifact(t, storageDir, "2024-01-15-10-30-00.sql.gz.enc", encrypted)

	cfg := restore.DefaultConfig()
	cfg.Storage.URL = "file://" + storageDir
	cfg.Encryption.EncryptionKey = base64.StdEncoding.EncodeToString(key)
	cfg.TargetDir = targetDir

	op := restore.NewOperation(cfg, logging.NewTestLogger())
	require.NoError(t, op.Run(context.Background(), "2024-01-15-10-30-00.sql.gz.enc"))

	assertFileContent(t, filepath.Join(targetDir, "db1.sql"), "dump of db1\n")
	assertFileContent(t, filepath.Join(targetDir, "db2.sql"), "dump of db2\n")
}

func TestOperation_Run_Archive(t *testing.T) {
	storageDir := t.TempDir()
	targetDir := t.TempDir()

	writeArtifact(t, storageDir, "2024-01-15-10-30-00.tar.gz", tarGzBytes(t, map[string]string{
		"db1.sql": "dump of db1\n",
		"db2.sql": "dump of db2\n",
	}))

	cfg := restore.DefaultConfig()
	cfg.Storage.URL = "file://" + storageDir
	cfg.TargetDir = targetDir

	op := restore.NewOperation(cfg, logging.NewTestLogger())
	require.NoError(t, op.Run(context.Background(), "2024-01-15-10-30-00.tar.gz"))

	assertFileContent(t, filepath.Join(targetDir, "db1.sql"), "dump of db1\n")
	assertFileContent(t, filepath.Join(targetDir, "db2.sql"), "dump of db2\n")
}

func TestOperation_Run_SQLStream_NotGzip(t *testing.T) {
	storageDir := t.TempDir()
	targetDir := t.TempDir()

	writeArtifact(t, storageDir, "2024-01-15-10-30-00.sql.gz", []byte("not a gzip stream"))

	cfg := restore.DefaultConfig()
	cfg.Storage.URL = "file://" + storageDir
	cfg.TargetDir = targetDir

	op := restore.NewOperation(cfg, logging.NewTestLogger())
	err := op.Run(context.Background(), "2024-01-15-10-30-00.sql.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create gzip reader")
}

func writeArtifact(t *testing.T, dir string, name string, content []byte) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
}

func gzipBytes(t *testing.T, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return buf.Bytes()
}

func encryptBytes(t *testing.T, key []byte, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	service := encryption.NewAES256GCMService(key)
	require.NoError(t, service.Encrypt(context.Background(), bytes.NewReader(content), &buf))

	return buf.Bytes()
}

func tarGzBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		require.NoError(t, tarWriter.WriteHeader(header))
		_, err := tarWriter.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	return buf.Bytes()
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(content))
}
