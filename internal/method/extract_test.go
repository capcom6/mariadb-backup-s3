package method_test

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
)

func TestExtractTar_ExtractsArchive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := "CREATE TABLE users (id INT);"
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "dump.sql", Mode: 0o600, Size: int64(len(content))}))
	_, err := tw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	targetDir := t.TempDir()
	require.NoError(t, method.ExtractTar(context.Background(), &buf, targetDir, logging.NewTestLogger()))

	got, readErr := os.ReadFile(filepath.Join(targetDir, "dump.sql"))
	require.NoError(t, readErr)
	assert.Equal(t, content, string(got))
}

func TestExtractTar_InvalidStreamFails(t *testing.T) {
	t.Parallel()

	err := method.ExtractTar(
		context.Background(),
		strings.NewReader("not a tar archive"),
		t.TempDir(),
		logging.NewTestLogger(),
	)
	require.ErrorIs(t, err, method.ErrExternalCommandFailed)
	assert.Contains(t, err.Error(), "failed to run tar")
}
