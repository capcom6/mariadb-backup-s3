package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

func TestDefaultMariaDB(t *testing.T) {
	cfg := config.DefaultMariaDB()
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 3306, cfg.Port)
	assert.Equal(t, "root", cfg.User)
	assert.Equal(t, "mariadb-backup", cfg.BackupBinary)
	assert.Equal(t, "mariadb", cfg.ClientBinary)
	assert.Equal(t, config.BackupMethodPhysical, cfg.BackupMethod)
	assert.False(t, cfg.IsLogical())
}

func TestMariaDB_Validate_BinaryFound(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.MariaDB{
		BackupBinary: exe,
		BackupMethod: config.BackupMethodPhysical,
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestMariaDB_Validate_BinaryNotFound(t *testing.T) {
	cfg := config.MariaDB{
		BackupBinary: "nonexistent-binary-12345",
		BackupMethod: config.BackupMethodPhysical,
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestMariaDB_Validate_InvalidMethod(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.MariaDB{
		BackupBinary: exe,
		BackupMethod: "invalid-method",
	}
	err = cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrInvalidBackupMethod)
}

func TestMariaDB_Validate_LogicalMethod(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.MariaDB{
		BackupBinary: exe,
		ClientBinary: exe,
		BackupMethod: config.BackupMethodLogical,
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestMariaDB_Validate_LogicalMethod_ClientBinaryNotFound(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.MariaDB{
		BackupBinary: exe,
		ClientBinary: "nonexistent-client-binary-12345",
		BackupMethod: config.BackupMethodLogical,
	}
	err = cfg.Validate()
	assert.Error(t, err)
}

func TestMariaDB_IsLogical(t *testing.T) {
	physical := config.MariaDB{BackupMethod: config.BackupMethodPhysical}
	assert.False(t, physical.IsLogical())

	logical := config.MariaDB{BackupMethod: config.BackupMethodLogical}
	assert.True(t, logical.IsLogical())
}

func TestStorage_Validate_ValidURL(t *testing.T) {
	cfg := config.Storage{
		URL: "s3://bucket/path",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestStorage_Validate_InvalidURL(t *testing.T) {
	cfg := config.Storage{
		URL: "://",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestStorage_GetURL_Success(t *testing.T) {
	cfg := config.Storage{
		URL: "s3://mybucket/some/path",
	}
	u, err := cfg.GetURL()
	require.NoError(t, err)
	assert.Equal(t, "s3", u.Scheme)
	assert.Equal(t, "mybucket", u.Host)
	assert.Equal(t, "/some/path", u.Path)
}

func TestStorage_GetURL_Error(t *testing.T) {
	cfg := config.Storage{
		URL: "://",
	}
	_, err := cfg.GetURL()
	assert.Error(t, err)
}

func TestEncryption_Enabled_WithKey(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "dGVzdGtleQ==",
	}
	assert.True(t, cfg.Enabled())
}

func TestEncryption_Enabled_Empty(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "",
	}
	assert.False(t, cfg.Enabled())
}

func TestEncryption_Key_Success(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "dGVzdGtleQ==",
	}
	key, err := cfg.Key()
	require.NoError(t, err)
	assert.Equal(t, []byte("testkey"), key)
}

func TestEncryption_Key_InvalidEncoding(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "not-valid-base64!!!",
	}
	_, err := cfg.Key()
	assert.Error(t, err)
}
