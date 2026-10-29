package restore

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

const (
	archiveFileMode  = 0o600
	archiveName      = "backup.tar.gz"
	extractedMarker  = "dump.sql"
	extractedContent = "CREATE TABLE users (id INT);"
)

// writeTestArchive writes a gzipped tar archive containing the given files.
func writeTestArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     archiveFileMode,
			Size:     int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), archiveFileMode))
}

func storageURL(dir string) string {
	return "file://" + dir
}

func TestRun_ExtractOnly(t *testing.T) {
	binDir := t.TempDir()
	markerPath := filepath.Join(t.TempDir(), "mongorestore-ran")
	script := "#!/bin/sh\nprintf called > \"$MONGORESTORE_MARKER\"\nexit 99\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongorestore"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MONGORESTORE_MARKER", markerPath)

	storageDir := t.TempDir()
	targetDir := t.TempDir()
	writeTestArchive(t, filepath.Join(storageDir, archiveName), map[string]string{
		extractedMarker:    extractedContent,
		"testdb/dump.bson": "fake-bson-data",
	})

	cfg := Config{
		Storage:    config.Storage{URL: storageURL(storageDir)},
		Encryption: config.Encryption{EncryptionKey: ""},
		TargetDir:  targetDir,
	}

	require.NoError(t, NewOperation(cfg, logging.NewTestLogger()).Run(context.Background(), archiveName))

	data, readErr := os.ReadFile(filepath.Join(targetDir, extractedMarker))
	require.NoError(t, readErr)
	require.Equal(t, extractedContent, string(data))
	bson, bsonErr := os.ReadFile(filepath.Join(targetDir, "testdb/dump.bson"))
	require.NoError(t, bsonErr)
	require.Equal(t, "fake-bson-data", string(bson))
	_, markerErr := os.Stat(markerPath)
	assert.True(t, os.IsNotExist(markerErr), "restore must extract without invoking mongorestore")
}
