package artifact_test

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/method"
	"github.com/capcom6/mariadb-backup-s3/internal/method/artifact"
)

const (
	sqlStreamFixture = "-- mariadb-backup-s3: database=db1\ndump of db1\n" +
		"-- mariadb-backup-s3: database=db2\ndump of db2\n"
	mongoStreamFixture = "fake-mongodump-archive-data"
	tarStreamFile      = "dump.sql"
	tarStreamContent   = "CREATE TABLE users (id INT);"
)

// tarBytes builds a plain tar stream: artifact.Restore receives a stream that
// the restore pipeline has already decompressed.
func tarBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())

	return buf.Bytes()
}

func TestRestore_Dispatch(t *testing.T) {
	t.Parallel()

	tarStream := tarBytes(t, map[string]string{tarStreamFile: tarStreamContent})

	tests := []struct {
		name     string
		filename string
		stream   []byte
		expected map[string]string
	}{
		{
			name:     "sql stream",
			filename: "2024-01-15-10-30-00.sql.gz",
			stream:   []byte(sqlStreamFixture),
			expected: map[string]string{
				"db1.sql": "dump of db1\n",
				"db2.sql": "dump of db2\n",
			},
		},
		{
			name:     "sql stream encrypted",
			filename: "2024-01-15-10-30-00.sql.gz.enc",
			stream:   []byte(sqlStreamFixture),
			expected: map[string]string{
				"db1.sql": "dump of db1\n",
				"db2.sql": "dump of db2\n",
			},
		},
		{
			name:     "mongo stream",
			filename: "2024-01-15-10-30-00.mongo.gz",
			stream:   []byte(mongoStreamFixture),
			expected: map[string]string{
				"2024-01-15-10-30-00.mongo": mongoStreamFixture,
			},
		},
		{
			name:     "mongo stream encrypted",
			filename: "2024-01-15-10-30-00.mongo.gz.enc",
			stream:   []byte(mongoStreamFixture),
			expected: map[string]string{
				"2024-01-15-10-30-00.mongo": mongoStreamFixture,
			},
		},
		{
			name:     "mongo stream subdirectory artifact keeps base name",
			filename: "prefix/2024-01-15-10-30-00.mongo.gz",
			stream:   []byte(mongoStreamFixture),
			expected: map[string]string{
				"2024-01-15-10-30-00.mongo": mongoStreamFixture,
			},
		},
		{
			name:     "tar archive",
			filename: "2024-01-15-10-30-00.tar.gz",
			stream:   tarStream,
			expected: map[string]string{tarStreamFile: tarStreamContent},
		},
		{
			name:     "tar archive encrypted",
			filename: "2024-01-15-10-30-00.tar.gz.enc",
			stream:   tarStream,
			expected: map[string]string{tarStreamFile: tarStreamContent},
		},
		{
			name:     "unknown extension defaults to tar",
			filename: "2024-01-15-10-30-00.unknown",
			stream:   tarStream,
			expected: map[string]string{tarStreamFile: tarStreamContent},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			err := artifact.Restore(
				context.Background(),
				bytes.NewReader(tt.stream),
				targetDir,
				tt.filename,
				logging.NewTestLogger(),
			)
			require.NoError(t, err)

			for name, want := range tt.expected {
				got, readErr := os.ReadFile(filepath.Join(targetDir, name))
				require.NoError(t, readErr)
				assert.Equal(t, want, string(got))
			}
		})
	}
}

func TestRestore_SQLStreamWithoutMarkerFails(t *testing.T) {
	t.Parallel()

	err := artifact.Restore(
		context.Background(),
		strings.NewReader("SELECT 1;\n"),
		t.TempDir(),
		"2024-01-15-10-30-00.sql.gz",
		logging.NewTestLogger(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to split the backup")
}

func TestRestore_InvalidTarFails(t *testing.T) {
	t.Parallel()

	err := artifact.Restore(
		context.Background(),
		strings.NewReader("not a tar archive"),
		t.TempDir(),
		"2024-01-15-10-30-00.tar.gz",
		logging.NewTestLogger(),
	)
	require.ErrorIs(t, err, method.ErrExternalCommandFailed)
	assert.Contains(t, err.Error(), "failed to run tar")
}
