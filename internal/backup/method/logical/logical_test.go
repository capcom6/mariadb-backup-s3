package logical_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/logical"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

func TestNew(t *testing.T) {
	cfg := method.Config{
		Host:         "localhost",
		Port:         3306,
		User:         "root",
		Password:     "secret",
		BackupBinary: "mariadb-dump",
	}

	m := logical.New(cfg)
	assert.NotNil(t, m)
}

func TestMethod_MethodName(t *testing.T) {
	cfg := method.Config{
		BackupBinary: "mariadb-dump",
	}

	m := logical.New(cfg)
	assert.Equal(t, logical.MethodName, m.MethodName())
}

func TestMethod_Prepare_NoOp(t *testing.T) {
	cfg := method.Config{
		BackupBinary: "mariadb-dump",
	}

	m := logical.New(cfg)
	logger := logging.NewTestLogger()
	err := m.Prepare(context.Background(), "/tmp/nonexistent", logger)
	require.NoError(t, err)
}

func TestMethod_ArtifactExtension(t *testing.T) {
	fileMode := logical.New(method.Config{BackupBinary: "mariadb-dump"})
	assert.Equal(t, method.ArtifactTarGz, fileMode.ArtifactExtension())

	streamMode := logical.New(method.Config{BackupBinary: "mariadb-dump", Stream: true})
	assert.Equal(t, "sql.gz", streamMode.ArtifactExtension())
}

func TestMethod_Backup_DisabledInStreamMode(t *testing.T) {
	m := logical.New(method.Config{BackupBinary: "mariadb-dump", Stream: true})

	err := m.Backup(context.Background(), t.TempDir(), logging.NewTestLogger())
	require.Error(t, err)
	require.ErrorIs(t, err, logical.ErrInvalidMode)
}

func TestMethod_Stream_DisabledWithoutStreamMode(t *testing.T) {
	m := logical.New(method.Config{BackupBinary: "mariadb-dump"})

	var buf strings.Builder
	err := m.Stream(context.Background(), &buf, logging.NewTestLogger())
	require.Error(t, err)
	require.ErrorIs(t, err, logical.ErrInvalidMode)
	assert.Empty(t, buf.String())
}

func TestMethod_Stream_WritesMarkerPerDatabase(t *testing.T) {
	cfg := method.Config{
		User:         "root",
		BackupBinary: writeStub(t, "mariadb-dump", "printf 'dump of %s\\n' \"$2\""),
		ClientBinary: writeStub(t, "mariadb", "printf 'db1\\ndb2\\n'"),
		Stream:       true,
	}

	m := logical.New(cfg)

	var buf strings.Builder
	require.NoError(t, m.Stream(context.Background(), &buf, logging.NewTestLogger()))

	assert.Equal(t, "-- mariadb-backup-s3: database=db1\ndump of db1\n"+
		"-- mariadb-backup-s3: database=db2\ndump of db2\n", buf.String())
}

func TestMethod_Stream_NoDatabases(t *testing.T) {
	cfg := method.Config{
		User:         "root",
		BackupBinary: writeStub(t, "mariadb-dump", "printf 'dump\\n'"),
		ClientBinary: writeStub(t, "mariadb", "printf ''"),
		Stream:       true,
	}

	m := logical.New(cfg)

	var buf strings.Builder
	require.NoError(t, m.Stream(context.Background(), &buf, logging.NewTestLogger()))
	assert.Empty(t, buf.String())
}

func TestMethod_Stream_DumpFails(t *testing.T) {
	cfg := method.Config{
		User:         "root",
		BackupBinary: writeStub(t, "mariadb-dump", "printf 'partial dump\\n'; exit 1"),
		ClientBinary: writeStub(t, "mariadb", "printf 'db1\\n'"),
		Stream:       true,
	}

	m := logical.New(cfg)

	var buf strings.Builder
	err := m.Stream(context.Background(), &buf, logging.NewTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to dump database \"db1\"")
}

// writeStub creates an executable shell script in a temporary directory and
// returns its path.
func writeStub(t *testing.T, name string, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	content := "#!/bin/sh\n" + body + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o700))

	return path
}
