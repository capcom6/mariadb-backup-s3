package mongodb_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/method/mongodb"
)

const extractStreamFixture = "fake-mongodump-archive-data"

func TestExtractStream_WritesSingleFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		wantName string
	}{
		{name: "gz suffix stripped", filename: "2024-01-15-10-30-00.mongo.gz", wantName: "2024-01-15-10-30-00.mongo"},
		{
			name:     "enc suffix stripped",
			filename: "2024-01-15-10-30-00.mongo.gz.enc",
			wantName: "2024-01-15-10-30-00.mongo",
		},
		{name: "subdirectory base name", filename: "prefix/x.mongo.gz", wantName: "x.mongo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()
			stream := strings.NewReader(extractStreamFixture)

			require.NoError(t, mongodb.ExtractStream(
				stream,
				targetDir,
				tt.filename,
			))

			got, readErr := os.ReadFile(filepath.Join(targetDir, tt.wantName))
			require.NoError(t, readErr)
			assert.Equal(t, extractStreamFixture, string(got))

			entries, dirErr := os.ReadDir(targetDir)
			require.NoError(t, dirErr)
			require.Len(t, entries, 1)
		})
	}
}

func TestExtractStream_WriteErrorFails(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	err := mongodb.ExtractStream(
		failingReader{},
		targetDir,
		"2024-01-15-10-30-00.mongo.gz",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write archive file")

	entries, dirErr := os.ReadDir(targetDir)
	require.NoError(t, dirErr)
	assert.Empty(t, entries, "no partial archive should remain after a write error")
}

func TestExtractStream_ReplacesSymlinkDestination(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "outside-file")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))

	dest := filepath.Join(targetDir, "2024-01-15-10-30-00.mongo")
	require.NoError(t, os.Symlink(victim, dest))

	require.NoError(t, mongodb.ExtractStream(
		strings.NewReader(extractStreamFixture),
		targetDir,
		"2024-01-15-10-30-00.mongo.gz",
	))

	got, readErr := os.ReadFile(dest)
	require.NoError(t, readErr)
	assert.Equal(t, extractStreamFixture, string(got))

	info, statErr := os.Lstat(dest)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "destination must be a regular file, not a symlink")

	victimData, victimErr := os.ReadFile(victim)
	require.NoError(t, victimErr)
	assert.Equal(t, "victim", string(victimData), "symlink target must stay untouched")
}

func TestExtractStream_FilePermissions(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	dest := filepath.Join(targetDir, "2024-01-15-10-30-00.mongo")

	require.NoError(t, mongodb.ExtractStream(
		strings.NewReader(extractStreamFixture),
		targetDir,
		"2024-01-15-10-30-00.mongo.gz",
	))

	info, statErr := os.Stat(dest)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// failingReader simulates a stream that breaks mid-copy.
type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, assert.AnError
}
