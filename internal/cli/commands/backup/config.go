package backup

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/urfave/cli/v3"
)

func parseConfig(cmd *cli.Command) (backup.Config, error) {
	cfg := backup.DefaultConfig()

	cfg.Version = cmd.Root().Version

	// Configure MariaDB settings
	cfg.MariaDB.Host = cmd.String("db-host")
	cfg.MariaDB.Port = cmd.Int("db-port")
	cfg.MariaDB.User = cmd.String("db-user")
	cfg.MariaDB.Password = cmd.String("db-password")
	cfg.MariaDB.BackupOptions = cmd.String("db-backup-options")
	cfg.MariaDB.BackupBinary = cmd.String("db-backup-binary")

	// Configure storage
	cfg.Storage.URL = cmd.String("storage-url")

	// Configure encryption
	cfg.Encryption.EncryptionKey = cmd.String("encryption-key")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return backup.Config{}, fmt.Errorf("failed to parse backup configuration: %w", err)
	}

	return cfg, nil
}
