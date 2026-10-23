package restore

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

var ErrValidationFailed = errors.New("validation failed")

type Config struct {
	Storage    config.Storage
	Encryption config.Encryption
	Database   config.Database

	TargetDir string
}

func (c Config) Validate() error {
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("%w: storage validation failed: %w", ErrValidationFailed, err)
	}

	if c.TargetDir == "" {
		return fmt.Errorf("%w: target directory is empty", ErrValidationFailed)
	}

	return c.validateDatabase()
}

// validateDatabase checks the engine-specific restore requirements. MariaDB
// restore is extract-only and never invokes a binary; MongoDB restore runs an
// external tool, so its connection and restore binary must be usable.
func (c Config) validateDatabase() error {
	engine := c.Database.Engine
	if engine == "" {
		engine = config.EngineMariaDB
	}

	switch engine {
	case config.EngineMariaDB:
		return nil
	case config.EngineMongoDB:
		return c.validateMongoDB(engine)
	default:
		return fmt.Errorf(
			"%w: database validation failed: %w: %q",
			ErrValidationFailed, config.ErrInvalidEngine, engine,
		)
	}
}

func (c Config) validateMongoDB(engine string) error {
	if c.Database.MongoDB.URI == "" && c.Database.Host == "" {
		return fmt.Errorf(
			"%w: database validation failed: mongodb restore requires a host or mongo URI",
			ErrValidationFailed,
		)
	}

	if c.Database.RestoreBinary == "" {
		return fmt.Errorf(
			"%w: database validation failed: restore binary is required for engine %q",
			ErrValidationFailed, engine,
		)
	}

	if _, err := exec.LookPath(c.Database.RestoreBinary); err != nil {
		return fmt.Errorf(
			"%w: database validation failed: binary '%s' not found in PATH: %w",
			ErrValidationFailed, c.Database.RestoreBinary, err,
		)
	}

	return nil
}

func DefaultConfig() Config {
	return Config{
		Storage: config.Storage{
			URL: "",
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
		Database: config.DefaultDatabase(config.EngineMariaDB),

		TargetDir: "",
	}
}
