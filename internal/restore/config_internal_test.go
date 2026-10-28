package restore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

func validConfig() Config {
	return Config{
		Storage:    config.Storage{URL: "file:///tmp/storage"},
		Encryption: config.Encryption{EncryptionKey: ""},
		TargetDir:  "/tmp/target",
	}
}

func TestConfigValidate_MariaDBNeverLooksUpBinary(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Database = config.Database{
		Engine:        config.EngineMariaDB,
		RestoreBinary: "definitely-not-in-path-12345",
	}

	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_MongoDBMissingRestoreBinary(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Database = config.Database{
		Engine: config.EngineMongoDB,
		Host:   "localhost",
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidationFailed)
	assert.Contains(t, err.Error(), "restore binary is required")
}

func TestConfigValidate_MongoDBRestoreBinaryNotInPath(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Database = config.Database{
		Engine:        config.EngineMongoDB,
		Host:          "localhost",
		RestoreBinary: "mongorestore-not-in-path-12345",
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidationFailed)
	assert.Contains(t, err.Error(), "not found in PATH")
}

func TestConfigValidate_MongoDBMissingConnection(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Database = config.Database{
		Engine:        config.EngineMongoDB,
		RestoreBinary: "mongorestore",
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidationFailed)
	assert.Contains(t, err.Error(), "host")
}

func TestConfigValidate_MongoDBValidWithRestoreBinaryInPath(t *testing.T) {
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongorestore"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := validConfig()
	cfg.Database = config.Database{
		Engine:        config.EngineMongoDB,
		Host:          "localhost",
		RestoreBinary: "mongorestore",
	}

	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_MongoDBValidWithURIOnly(t *testing.T) {
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongorestore"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := validConfig()
	cfg.Database = config.Database{
		Engine:        config.EngineMongoDB,
		RestoreBinary: "mongorestore",
		MongoDB:       config.MongoDB{URI: "mongodb://mongo1:27017"},
	}

	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_EmptyEngineDefaultsToMariaDB(t *testing.T) {
	t.Parallel()
	cfg := validConfig()

	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_InvalidEngine(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Database = config.Database{Engine: "mysql"}

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidationFailed)
	require.ErrorIs(t, err, config.ErrInvalidEngine)
}

func TestConfigValidate_TargetDirRequired(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.TargetDir = ""

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidationFailed)
	assert.Contains(t, err.Error(), "target directory is empty")
}
