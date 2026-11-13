package mongodb_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
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
				context.Background(),
				stream,
				targetDir,
				tt.filename,
				logging.NewTestLogger(),
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

	err := mongodb.ExtractStream(
		context.Background(),
		failingReader{},
		t.TempDir(),
		"2024-01-15-10-30-00.mongo.gz",
		logging.NewTestLogger(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write archive file")
}

// failingReader simulates a stream that breaks mid-copy.
type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, assert.AnError
}
