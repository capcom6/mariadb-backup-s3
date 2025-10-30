package backup

import (
	"errors"
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

var ErrValidationFailed = errors.New("validation failed")

type LimitsConfig struct {
	MaxCount int
}

type Backup struct {
	Limits LimitsConfig
}

type Config struct {
	MariaDB    config.MariaDB
	Storage    config.Storage
	Backup     Backup
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
		MariaDB: config.DefaultMariaDB(),
		Storage: config.Storage{
			URL: "",
		},
		Backup: Backup{
			Limits: LimitsConfig{MaxCount: 0},
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
	}
}
