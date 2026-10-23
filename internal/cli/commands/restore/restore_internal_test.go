package restore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
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
	sel, err := runResolveSelection(t, "backup.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, "backup.tar.gz", sel.filename)
	assert.Empty(t, sel.method)

	engine, conflict := resolveEngine("", sel.method)
	assert.Equal(t, config.EngineMariaDB, engine)
	assert.False(t, conflict)
}

func TestResolveBackupSelection_MissingSelector(t *testing.T) {
	_, err := runResolveSelection(t)
	require.ErrorIs(t, err, ErrInvalidParams)
}

func TestResolveBackupSelection_MultipleSelectors(t *testing.T) {
	_, err := runResolveSelection(t, "--latest", "backup.tar.gz")
	require.ErrorIs(t, err, ErrInvalidParams)
}

func TestResolveBackupSelection_LatestDerivesEngineFromRegistry(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedEngine string
	}{
		{
			name:           "mongodump selects mongodb",
			method:         config.BackupMethodMongoDB,
			expectedEngine: config.EngineMongoDB,
		},
		{
			name:           "mariadb-dump selects mariadb",
			method:         config.BackupMethodLogical,
			expectedEngine: config.EngineMariaDB,
		},
		{
			name:           "mariadb-backup selects mariadb",
			method:         config.BackupMethodPhysical,
			expectedEngine: config.EngineMariaDB,
		},
		{
			name:           "empty method defaults to mariadb",
			method:         "",
			expectedEngine: config.EngineMariaDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storageURL := writeRegistry(t, newReadyEntry(t, tt.method))

			sel, err := runResolveSelection(t, "--storage-url="+storageURL, "--latest")
			require.NoError(t, err)
			assert.Equal(t, registryBackupName, sel.filename)
			assert.Equal(t, tt.method, sel.method)

			engine, conflict := resolveEngine("", sel.method)
			assert.Equal(t, tt.expectedEngine, engine)
			assert.False(t, conflict)
		})
	}
}

func TestResolveBackupSelection_BackupIDDerivesEngineFromRegistry(t *testing.T) {
	entry := newReadyEntry(t, config.BackupMethodMongoDB)
	storageURL := writeRegistry(t, entry)

	sel, err := runResolveSelection(t, "--storage-url="+storageURL, "--backup-id="+entry.ID)
	require.NoError(t, err)
	assert.Equal(t, entry.Filename, sel.filename)
	assert.Equal(t, config.BackupMethodMongoDB, sel.method)

	engine, conflict := resolveEngine("", sel.method)
	assert.Equal(t, config.EngineMongoDB, engine)
	assert.False(t, conflict)
}

func TestResolveBackupSelection_ExplicitEngineWins(t *testing.T) {
	storageURL := writeRegistry(t, newReadyEntry(t, config.BackupMethodMongoDB))

	sel, err := runResolveSelection(t, "--storage-url="+storageURL, "--latest")
	require.NoError(t, err)

	engine, conflict := resolveEngine(config.EngineMariaDB, sel.method)
	assert.Equal(t, config.EngineMariaDB, engine)
	assert.True(t, conflict, "explicit engine contradicting registry method must be reported")
}

func TestResolveBackupSelection_LatestWithoutReadyBackups(t *testing.T) {
	storageURL := writeRegistry(t)

	_, err := runResolveSelection(t, "--storage-url="+storageURL, "--latest")
	require.ErrorIs(t, err, ErrBackupNotFound)
}

func TestResolveBackupSelection_BackupIDNotFound(t *testing.T) {
	storageURL := writeRegistry(t, newReadyEntry(t, config.BackupMethodMongoDB))

	_, err := runResolveSelection(t, "--storage-url="+storageURL, "--backup-id=missing-id")
	require.ErrorIs(t, err, ErrBackupNotFound)
}

func TestResolveEngine(t *testing.T) {
	tests := []struct {
		name             string
		explicit         string
		method           string
		expectedEngine   string
		expectedConflict bool
	}{
		{
			name:             "explicit wins over registry method",
			explicit:         config.EngineMongoDB,
			method:           config.BackupMethodPhysical,
			expectedEngine:   config.EngineMongoDB,
			expectedConflict: true,
		},
		{
			name:             "explicit mariadb over mongodump",
			explicit:         config.EngineMariaDB,
			method:           config.BackupMethodMongoDB,
			expectedEngine:   config.EngineMariaDB,
			expectedConflict: true,
		},
		{
			name:             "explicit without registry method has no conflict",
			explicit:         config.EngineMongoDB,
			method:           "",
			expectedEngine:   config.EngineMongoDB,
			expectedConflict: false,
		},
		{
			name:             "derived from registry mongodump",
			explicit:         "",
			method:           config.BackupMethodMongoDB,
			expectedEngine:   config.EngineMongoDB,
			expectedConflict: false,
		},
		{
			name:             "derived from registry mariadb-dump",
			explicit:         "",
			method:           config.BackupMethodLogical,
			expectedEngine:   config.EngineMariaDB,
			expectedConflict: false,
		},
		{
			name:             "empty method defaults to mariadb",
			explicit:         "",
			method:           "",
			expectedEngine:   config.EngineMariaDB,
			expectedConflict: false,
		},
		{
			name:             "unknown method defaults to mariadb",
			explicit:         "",
			method:           "future-method",
			expectedEngine:   config.EngineMariaDB,
			expectedConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, conflict := resolveEngine(tt.explicit, tt.method)
			assert.Equal(t, tt.expectedEngine, engine)
			assert.Equal(t, tt.expectedConflict, conflict)
		})
	}
}
