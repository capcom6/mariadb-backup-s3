package backup

import (
	"errors"
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

var ErrValidationFailed = errors.New("validation failed")

type Config struct {
	Version    string
	MariaDB    config.MariaDB
	Storage    config.Storage
	Encryption config.Encryption

	// Stream makes the backup produce its output as a stream, instead of
	// writing files into a temp directory. Only logical backups support it.
	Stream bool
}

func (c Config) Validate() error {
	if err := c.MariaDB.Validate(); err != nil {
		return fmt.Errorf("%w: MariaDB validation failed: %w", ErrValidationFailed, err)
	}

	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("%w: storage validation failed: %w", ErrValidationFailed, err)
	}

	if c.Stream && !c.MariaDB.IsLogical() {
		return fmt.Errorf("%w: streaming is supported by the logical backup method only", ErrValidationFailed)
	}

	return nil
}

func DefaultConfig() Config {
	return Config{
		Version: "",
		MariaDB: config.DefaultMariaDB(),
		Storage: config.Storage{
			URL: "",
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
		Stream: false,
	}
}
