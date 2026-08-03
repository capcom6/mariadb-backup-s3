package backup

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
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
	cfg.MariaDB.BackupMethod = cmd.String("backup-method")

	// Auto-derive binary from method if not explicitly set
	cfg.MariaDB.BackupBinary = cmd.String("db-backup-binary")
	if cfg.MariaDB.BackupBinary == "" {
		cfg.MariaDB.BackupBinary = defaultBinaryForMethod(cfg.MariaDB.BackupMethod)
	}

	// Client binary defaults to mariadb
	cfg.MariaDB.ClientBinary = cmd.String("db-client-binary")
	if cfg.MariaDB.ClientBinary == "" {
		cfg.MariaDB.ClientBinary = "mariadb"
	}

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

func defaultBinaryForMethod(method string) string {
	switch method {
	case config.BackupMethodLogical:
		return "mariadb-dump"
	default:
		return "mariadb-backup"
	}
}
