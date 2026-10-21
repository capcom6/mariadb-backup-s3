package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

func TestConfig_Validate_RejectsStreamWithPhysicalMethod(t *testing.T) {
	cfg := backup.DefaultConfig()
	cfg.MariaDB.BackupMethod = config.BackupMethodPhysical
	cfg.MariaDB.BackupBinary = writeStubBinary(t, "mariadb-backup")
	cfg.Storage.URL = "s3://bucket/prefix"
	cfg.Stream = true

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, backup.ErrValidationFailed)
	assert.Contains(t, err.Error(), "streaming is supported by the logical backup method only")
}

func TestConfig_Validate_AcceptsStreamWithLogicalMethod(t *testing.T) {
	cfg := backup.DefaultConfig()
	cfg.MariaDB.BackupMethod = config.BackupMethodLogical
	cfg.MariaDB.BackupBinary = writeStubBinary(t, "mariadb-dump")
	cfg.MariaDB.ClientBinary = writeStubBinary(t, "mariadb")
	cfg.Storage.URL = "s3://bucket/prefix"
	cfg.Stream = true

	require.NoError(t, cfg.Validate())
}

func TestConfig_Validate_AcceptsFileModeWithPhysicalMethod(t *testing.T) {
	cfg := backup.DefaultConfig()
	cfg.MariaDB.BackupMethod = config.BackupMethodPhysical
	cfg.MariaDB.BackupBinary = writeStubBinary(t, "mariadb-backup")
	cfg.Storage.URL = "s3://bucket/prefix"

	require.NoError(t, cfg.Validate())
}

// writeStubBinary creates an executable stub in a temporary directory and
// returns its path.
func writeStubBinary(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700))

	return path
}
