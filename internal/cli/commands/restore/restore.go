package restore

import (
	"context"
	"errors"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/restore"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	fl := flags.Storage()
	fl = append(fl, flags.Encryption()...)
	fl = append(fl,
		&cli.StringFlag{
			Name:     "target-dir",
			Usage:    "target directory",
			Required: true,
			Sources:  cli.EnvVars("RESTORE__TARGET_DIR"),
		},
	)

	return &cli.Command{
		Name:  "restore",
		Usage: "Restore MariaDB database from backup",
		Flags: fl,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "backup-file",
				UsageText: "backup file name",
				Config: cli.StringConfig{
					TrimSpace: true,
				},
			},
		},
		ArgsUsage: "backup_name.tar.gz",
		Action: func(c context.Context, cmd *cli.Command) error {
			cfg := restore.DefaultConfig()

			cfg.Storage.URL = cmd.String("storage-url")

			cfg.Encryption.EncryptionKey = cmd.String("encryption-key")

			cfg.Restore.TargetDir = cmd.String("target-dir")

			if cmd.StringArg("backup-file") == "" {
				return errors.New("backup file name is required")
			}

			return restore.Execute(c, cmd.StringArg("backup-file"), cfg)
		},
	}
}
