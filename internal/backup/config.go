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
}

func (c Config) Validate() error {
	if err := c.MariaDB.Validate(); err != nil {
		return fmt.Errorf("%w: MariaDB validation failed: %w", ErrValidationFailed, err)
	}

	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("%w: storage validation failed: %w", ErrValidationFailed, err)
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
	}
}
