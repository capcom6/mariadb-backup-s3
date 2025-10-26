package restore

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
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
		Name:    "restore",
		Aliases: []string{"r"},
		Usage:   "Restore MariaDB database from backup",
		Flags:   fl,
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logger := logging.GetLogger(ctx)
			if logger == nil {
				return cli.Exit("failed to retrieve logger", codes.InternalError)
			}

			operationID := logging.GenerateOperationID("restore")
			logger = logger.WithContext("restore-cmd", operationID)

			logger.Info(ctx, "Restore command initiated")

			backupFile := cmd.StringArg("backup-file")
			if backupFile == "" {
				err := cli.Exit("backup file name is required", codes.ParamsError)
				logger.Error(ctx, "Backup file name is required", err)
				return err
			}

			targetDir := cmd.String("target-dir")
			if targetDir == "" {
				err := cli.Exit("target directory is required", codes.ParamsError)
				logger.Error(ctx, "Target directory is required", err)
				return err
			}

			// Log command parameters for debugging
			logger.Debug(ctx, "Parsing command parameters", logging.Fields{
				"backup_file":    backupFile,
				"target_dir":     targetDir,
				"storage_url":    cmd.String("storage-url"),
				"encryption_key": "***", // Don't log actual key
			})

			cfg := restore.DefaultConfig()

			cfg.Storage.URL = cmd.String("storage-url")
			cfg.Encryption.EncryptionKey = cmd.String("encryption-key")
			cfg.TargetDir = targetDir

			logger.Info(ctx, "Starting restore execution", logging.Fields{
				"backup_file":        backupFile,
				"target_dir":         cfg.TargetDir,
				"storage_url":        cfg.Storage.URL,
				"encryption_enabled": cfg.Encryption.Enabled(),
			})

			if err := restore.NewOperation(cfg, logger).Run(ctx, backupFile); err != nil {
				logger.Error(ctx, "Restore command failed", err)
				return cli.Exit("restore command failed", codes.InternalError)
			}

			logger.Info(ctx, "Restore command completed successfully")
			return nil
		},
	}
}
