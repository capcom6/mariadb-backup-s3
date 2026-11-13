package logical_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/method/logical"
)

const extractStreamFixture = "-- mariadb-backup-s3: database=db1\ndump of db1\n" +
	"-- mariadb-backup-s3: database=db2\ndump of db2\n"

func TestExtractStream_SplitsDatabases(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	stream := strings.NewReader(extractStreamFixture)

	require.NoError(t, logical.ExtractStream(context.Background(), stream, targetDir, logging.NewTestLogger()))

	for db, want := range map[string]string{"db1.sql": "dump of db1\n", "db2.sql": "dump of db2\n"} {
		got, readErr := os.ReadFile(filepath.Join(targetDir, db))
		require.NoError(t, readErr)
		assert.Equal(t, want, string(got))
	}
}

func TestExtractStream_WithoutMarkerFails(t *testing.T) {
	t.Parallel()

	err := logical.ExtractStream(
		context.Background(),
		strings.NewReader("SELECT 1;\n"),
		t.TempDir(),
		logging.NewTestLogger(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to split the backup")
}
