package restore

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/restore"
	"github.com/urfave/cli/v3"
)

func parseConfig(cmd *cli.Command) (restore.Config, error) {
	cfg := restore.DefaultConfig()

	cfg.Storage.URL = cmd.String("storage-url")
	cfg.Encryption.EncryptionKey = cmd.String("encryption-key")
	cfg.TargetDir = cmd.String("target-dir")

	if err := cfg.Validate(); err != nil {
		return restore.Config{}, fmt.Errorf("failed to parse restore configuration: %w", err)
	}

	return cfg, nil
}
