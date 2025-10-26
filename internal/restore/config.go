package restore

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

type Config struct {
	Storage    config.Storage
	Encryption config.Encryption

	TargetDir string
}

func (c Config) Validate() error {
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("%w: storage validation failed: %w", ErrValidationFailed, err)
	}

	if c.TargetDir == "" {
		return fmt.Errorf("%w: target directory is empty", ErrValidationFailed)
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

		TargetDir: "",
	}
}
