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

const (
	logFieldTargetDir         = "target_dir"
	logFieldStorageURL        = "storage_url"
	logFieldEncryptionEnabled = "encryption_enabled"
)

type backupSelection struct{ filename string }

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
			Usage: "extract latest ready backup from registry",
		},
		&cli.StringFlag{
			Name:  "backup-id",
			Usage: "extract backup by registry backup ID",
		},
	)

	return &cli.Command{
		Name:    "restore",
		Aliases: []string{"r"},
		Usage:   "Extract backup files to a directory",
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
				logFieldTargetDir:  cmd.String("target-dir"),
				logFieldStorageURL: cmd.String("storage-url"),
				"encryption_key":   "***", // Don't log actual key
			})

			selection, selErr := resolveBackupSelection(ctx, cmd)
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

			cfg, err := parseConfig(cmd)
			if err != nil {
				logger.Error(ctx, "Restore command failed", err)
				return cli.Exit("restore command failed", codes.ParamsError)
			}

			logRestorePlan(ctx, logger, cfg, selection)

			if runErr := runRestoreOperation(ctx, cfg, logger, selection.filename); runErr != nil {
				logger.Error(ctx, "Restore command failed", runErr)
				return cli.Exit("restore command failed", codes.InternalError)
			}

			logger.Info(ctx, "Restore command completed successfully")
			return nil
		},
	}
}

// logRestorePlan logs the validated configuration and the execution plan.
func logRestorePlan(
	ctx context.Context,
	logger logging.Logger,
	cfg restore.Config,
	selection backupSelection,
) {
	logger.Debug(ctx, "Configuration validated successfully", logging.Fields{
		logFieldTargetDir:         cfg.TargetDir,
		logFieldStorageURL:        cfg.Storage.URL,
		logFieldEncryptionEnabled: cfg.Encryption.Enabled(),
	})

	logger.Info(ctx, "Starting restore execution", logging.Fields{
		"backup_file":             selection.filename,
		logFieldTargetDir:         cfg.TargetDir,
		logFieldStorageURL:        cfg.Storage.URL,
		logFieldEncryptionEnabled: cfg.Encryption.Enabled(),
	})
}

// runRestoreOperation creates the engine-specific restore operation and runs it.
func runRestoreOperation(ctx context.Context, cfg restore.Config, logger logging.Logger, filename string) error {
	if runErr := restore.NewOperation(cfg, logger).Run(ctx, filename); runErr != nil {
		return fmt.Errorf("restore operation failed: %w", runErr)
	}

	return nil
}

func resolveBackupSelection(ctx context.Context, cmd *cli.Command) (backupSelection, error) {
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
		return backupSelection{}, fmt.Errorf(
			"%w: specify one of: backup filename argument, --latest, or --backup-id",
			ErrInvalidParams,
		)
	}
	if selected > 1 {
		return backupSelection{}, fmt.Errorf(
			"%w: use only one selector: backup filename argument, --latest, or --backup-id",
			ErrInvalidParams,
		)
	}
	if backupFile != "" {
		return backupSelection{filename: backupFile}, nil
	}

	storageURL := cmd.String("storage-url")
	u, err := (config.Storage{URL: storageURL}).GetURL()
	if err != nil {
		return backupSelection{}, fmt.Errorf("failed to parse storage URL: %w", err)
	}
	storageSvc, err := storage.New(u)
	if err != nil {
		return backupSelection{}, fmt.Errorf("failed to initialize storage backend: %w", err)
	}
	defer func() {
		_ = storageSvc.Close()
	}()

	registrySvc := registry.NewService(storageSvc, registry.WithRecovery())
	reg, err := registrySvc.Load(ctx)
	if err != nil {
		return backupSelection{}, fmt.Errorf("failed to load registry: %w", err)
	}

	if latest {
		reg.SortByCreatedAtDesc()
		for _, b := range reg.Backups {
			if b.Status == registry.StatusReady {
				return backupSelection{filename: b.Filename}, nil
			}
		}
		return backupSelection{}, fmt.Errorf("%w: no ready backups found in registry", ErrBackupNotFound)
	}

	for _, b := range reg.Backups {
		if b.ID == backupID && b.Status == registry.StatusReady {
			return backupSelection{filename: b.Filename}, nil
		}
	}

	return backupSelection{}, fmt.Errorf("%w: backup-id not found in ready registry entries", ErrBackupNotFound)
}
