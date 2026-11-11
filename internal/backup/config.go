package backup

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

type Config struct {
	Version    string
	Database   config.Database
	Storage    config.Storage
	Encryption config.Encryption

	// Stream makes the backup produce its output as a stream, instead of
	// writing files into a temp directory. Supported by the logical
	// (mariadb-dump) and mongodump methods.
	Stream bool
}

func (c Config) Validate() error {
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("%w: database validation failed: %w", ErrValidationFailed, err)
	}

	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("%w: storage validation failed: %w", ErrValidationFailed, err)
	}

	return nil
}

func DefaultConfig() Config {
	return Config{
		Version:  "",
		Database: config.DefaultDatabase(config.EngineMariaDB),
		Storage: config.Storage{
			URL: "",
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
		Stream: false,
	}
}
