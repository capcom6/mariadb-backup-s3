package backup

import (
	"errors"
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

var ErrValidationFailed = errors.New("validation failed")

type Config struct {
	Version    string
	Database   config.Database
	Storage    config.Storage
	Encryption config.Encryption

	// Stream makes the backup produce its output as a stream, instead of
	// writing files into a temp directory. Only logical backups support it.
	Stream bool
}

func (c Config) Validate() error {
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("%w: database validation failed: %w", ErrValidationFailed, err)
	}

	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("%w: storage validation failed: %w", ErrValidationFailed, err)
	}

	if c.Stream && !c.Database.IsLogical() {
		return fmt.Errorf("%w: streaming is supported by the logical backup method only", ErrValidationFailed)
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
