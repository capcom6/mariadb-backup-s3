package restore

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestRun_MariaDBExtractOnly proves the historical MariaDB behavior: the
// pipeline downloads, decrypts nothing, and extracts into target-dir. No
// database binary is looked up or executed, even when RestoreBinary points
// at a path that does not exist.
func TestRun_MariaDBExtractOnly(t *testing.T) {
	storageDir := t.TempDir()
	targetDir := t.TempDir()
	writeTestArchive(t, filepath.Join(storageDir, archiveName), map[string]string{
		extractedMarker: extractedContent,
	})

	cfg := Config{
		Storage:    config.Storage{URL: storageURL(storageDir)},
		Encryption: config.Encryption{EncryptionKey: ""},
		Database: config.Database{
			Engine:        config.EngineMariaDB,
			RestoreBinary: "/nonexistent/restore-binary-must-not-run",
		},
		TargetDir: targetDir,
	}

	op, err := NewOperation(cfg, logging.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, op.Run(context.Background(), archiveName))

	data, readErr := os.ReadFile(filepath.Join(targetDir, extractedMarker))
	require.NoError(t, readErr)
	assert.Equal(t, extractedContent, string(data))
}

// TestRun_MongoDBRestoresAfterExtraction runs a full download-extract-restore
// cycle against a fake mongorestore. The fake fails unless the extracted dump
// already exists when it starts, so a passing run proves the restore hook runs
// after the pipeline and receives --dir=<target-dir>.
func TestRun_MongoDBRestoresAfterExtraction(t *testing.T) {
	binDir := t.TempDir()
	workDir := t.TempDir()
	argsFile := filepath.Join(workDir, "args")

	script := `#!/bin/sh
set -e
dir=""
for arg in "$@"; do
	case "$arg" in
	--dir=*) dir="${arg#--dir=}" ;;
	esac
done
if [ -z "$dir" ] || [ ! -f "$dir/testdb/dump.bson" ]; then
	echo "extraction not finished before mongorestore" >&2
	exit 9
fi
: > "$FAKE_MONGORESTORE_ARGS"
for arg in "$@"; do
	printf '%s\n' "$arg" >> "$FAKE_MONGORESTORE_ARGS"
done
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongorestore"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_MONGORESTORE_ARGS", argsFile)

	storageDir := t.TempDir()
	targetDir := t.TempDir()
	writeTestArchive(t, filepath.Join(storageDir, archiveName), map[string]string{
		"testdb/dump.bson": "fake-bson-data",
	})

	cfg := Config{
		Storage:    config.Storage{URL: storageURL(storageDir)},
		Encryption: config.Encryption{EncryptionKey: ""},
		Database: config.Database{
			Engine:        config.EngineMongoDB,
			Host:          "localhost",
			Port:          27017,
			RestoreBinary: "mongorestore",
		},
		TargetDir: targetDir,
	}

	op, err := NewOperation(cfg, logging.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, op.Run(context.Background(), archiveName))

	argsData, readErr := os.ReadFile(argsFile)
	require.NoError(t, readErr)
	args := strings.Split(strings.TrimRight(string(argsData), "\n"), "\n")
	assert.Contains(t, args, "--dir="+targetDir)
	assert.Contains(t, args, "--host=localhost")
	assert.Contains(t, args, "--port=27017")
}

func TestNewOperation_UnknownEngine(t *testing.T) {
	_, err := NewOperation(Config{
		Database: config.Database{Engine: "oracle"},
	}, logging.NewTestLogger())

	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrInvalidEngine)
}
