package restore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/capcom6/mariadb-backup-s3/internal/registry"
)

const (
	registryFileName   = ".backup-registry.json"
	registryBackupName = "2026-10-23-10-00-00.tar.gz"
)

// runResolveSelection runs the real restore command with its Action replaced
// by a capture of resolveBackupSelection.
func runResolveSelection(t *testing.T, args ...string) (backupSelection, error) {
	t.Helper()

	var (
		sel    backupSelection
		selErr error
	)

	cmd := Command()
	cmd.Action = func(ctx context.Context, c *cli.Command) error {
		sel, selErr = resolveBackupSelection(ctx, c)
		return nil
	}

	fullArgs := append([]string{"restore", "--target-dir=" + t.TempDir()}, args...)
	if !slices.ContainsFunc(args, func(a string) bool {
		return strings.HasPrefix(a, "--storage-url")
	}) {
		fullArgs = append(fullArgs, "--storage-url=file://"+t.TempDir())
	}
	runErr := cmd.Run(context.Background(), fullArgs)
	require.NoError(t, runErr)

	return sel, selErr
}

func newReadyEntry(t *testing.T, method string) registry.BackupEntry {
	t.Helper()

	createdAt := time.Date(2026, time.October, 23, 10, 0, 0, 0, time.UTC)

	return registry.NewBackupEntry(
		registryBackupName,
		createdAt,
		2048,
		"0123456789abcdef",
		registry.StatusReady,
		false,
		nil,
		&registry.ToolMetadata{Name: "mariadb-backup-s3", Version: "1.0.0", Method: method},
	)
}

func writeRegistry(t *testing.T, entries ...registry.BackupEntry) string {
	t.Helper()

	dir := t.TempDir()
	reg := registry.New()
	reg.Backups = append(reg.Backups, entries...)

	data, err := json.Marshal(reg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, registryFileName), data, 0o600))

	return "file://" + dir
}

func TestResolveBackupSelection_FilenameDefaultsToMariaDB(t *testing.T) {
	t.Parallel()
	sel, err := runResolveSelection(t, "backup.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, "backup.tar.gz", sel.filename)
}

func TestResolveBackupSelection_MissingSelector(t *testing.T) {
	t.Parallel()
	_, err := runResolveSelection(t)
	require.ErrorIs(t, err, ErrInvalidParams)
}

func TestResolveBackupSelection_MultipleSelectors(t *testing.T) {
	t.Parallel()
	_, err := runResolveSelection(t, "--latest", "backup.tar.gz")
	require.ErrorIs(t, err, ErrInvalidParams)
}

func TestResolveBackupSelection_LatestIgnoresBackupMethod(t *testing.T) {
	t.Parallel()
	storageURL := writeRegistry(t, newReadyEntry(t, "mongodump"))

	sel, err := runResolveSelection(t, "--storage-url="+storageURL, "--latest")
	require.NoError(t, err)
	assert.Equal(t, registryBackupName, sel.filename)
}

func TestResolveBackupSelection_BackupIDIgnoresBackupMethod(t *testing.T) {
	t.Parallel()
	entry := newReadyEntry(t, "mongodump")
	storageURL := writeRegistry(t, entry)

	sel, err := runResolveSelection(t, "--storage-url="+storageURL, "--backup-id="+entry.ID)
	require.NoError(t, err)
	assert.Equal(t, entry.Filename, sel.filename)
}

func TestResolveBackupSelection_LatestWithoutReadyBackups(t *testing.T) {
	t.Parallel()
	storageURL := writeRegistry(t)

	_, err := runResolveSelection(t, "--storage-url="+storageURL, "--latest")
	require.ErrorIs(t, err, ErrBackupNotFound)
}

func TestResolveBackupSelection_BackupIDNotFound(t *testing.T) {
	t.Parallel()
	storageURL := writeRegistry(t, newReadyEntry(t, "mongodump"))

	_, err := runResolveSelection(t, "--storage-url="+storageURL, "--backup-id=missing-id")
	require.ErrorIs(t, err, ErrBackupNotFound)
}
