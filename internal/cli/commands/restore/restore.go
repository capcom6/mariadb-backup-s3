package restore

import (
	"context"
	"errors"
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/restore"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
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
		&cli.BoolFlag{
			Name:  "latest",
			Usage: "restore latest ready backup from registry",
		},
		&cli.StringFlag{
			Name:  "backup-id",
			Usage: "restore backup by registry backup ID",
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
		ArgsUsage: "[backup_name.tar.gz | --latest | --backup-id=<id>]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logger := logging.GetLogger(ctx)
			if logger == nil {
				return cli.Exit("failed to retrieve logger", codes.InternalError)
			}

			operationID := logging.GenerateOperationID("restore")
			logger = logger.WithContext("restore-cmd", operationID)

			logger.Info(ctx, "Restore command initiated")

			// Log command parameters for debugging
			logger.Debug(ctx, "Parsing command parameters", logging.Fields{
				"target_dir":     cmd.String("target-dir"),
				"storage_url":    cmd.String("storage-url"),
				"encryption_key": "***", // Don't log actual key
			})

			cfg, err := parseConfig(cmd)
			if err != nil {
				logger.Error(ctx, "Restore command failed", err)
				return cli.Exit("restore command failed", codes.ParamsError)
			}

			backupFile, selErr := resolveBackupSelection(ctx, cmd)
			if selErr != nil {
				if errors.Is(selErr, ErrInvalidParams) {
					err := cli.Exit(selErr.Error(), codes.ParamsError)
					logger.Error(ctx, "Failed to resolve backup selection", err)
					return err
				}

				err := cli.Exit(selErr.Error(), codes.InternalError)
				logger.Error(ctx, "Failed to resolve backup selection", err)
				return err
			}

			// Validate configuration (debug visibility)
			logger.Debug(ctx, "Configuration validated successfully", logging.Fields{
				"target_dir":         cfg.TargetDir,
				"storage_url":        cfg.Storage.URL,
				"encryption_enabled": cfg.Encryption.Enabled(),
			})

			logger.Info(ctx, "Starting restore execution", logging.Fields{
				"backup_file":        backupFile,
				"target_dir":         cfg.TargetDir,
				"storage_url":        cfg.Storage.URL,
				"encryption_enabled": cfg.Encryption.Enabled(),
			})

			if opErr := restore.NewOperation(cfg, logger).Run(ctx, backupFile); opErr != nil {
				logger.Error(ctx, "Restore command failed", opErr)
				return cli.Exit("restore command failed", codes.InternalError)
			}

			logger.Info(ctx, "Restore command completed successfully")
			return nil
		},
	}
}

func resolveBackupSelection(ctx context.Context, cmd *cli.Command) (string, error) {
	backupFile := cmd.StringArg("backup-file")
	latest := cmd.Bool("latest")
	backupID := cmd.String("backup-id")

	selected := 0
	if backupFile != "" {
		selected++
	}
	if latest {
		selected++
	}
	if backupID != "" {
		selected++
	}
	if selected == 0 {
		return "", fmt.Errorf(
			"%w: specify one of: backup filename argument, --latest, or --backup-id",
			ErrInvalidParams,
		)
	}
	if selected > 1 {
		return "", fmt.Errorf(
			"%w: use only one selector: backup filename argument, --latest, or --backup-id",
			ErrInvalidParams,
		)
	}
	if backupFile != "" {
		return backupFile, nil
	}

	storageURL := cmd.String("storage-url")
	u, err := (config.Storage{URL: storageURL}).GetURL()
	if err != nil {
		return "", fmt.Errorf("failed to parse storage URL: %w", err)
	}
	storageSvc, err := storage.New(u)
	if err != nil {
		return "", fmt.Errorf("failed to initialize storage backend: %w", err)
	}
	defer func() {
		_ = storageSvc.Close()
	}()

	registrySvc := registry.NewService(storageSvc, registry.WithRecovery())
	reg, err := registrySvc.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load registry: %w", err)
	}

	if latest {
		reg.SortByCreatedAtDesc()
		for _, b := range reg.Backups {
			if b.Status == registry.StatusReady {
				return b.Filename, nil
			}
		}
		return "", fmt.Errorf("%w: no ready backups found in registry", ErrBackupNotFound)
	}

	for _, b := range reg.Backups {
		if b.ID == backupID && b.Status == registry.StatusReady {
			return b.Filename, nil
		}
	}

	return "", fmt.Errorf("%w: backup-id not found in ready registry entries", ErrBackupNotFound)
}
