package retention

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/retention"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/urfave/cli/v3"
)

const (
	logFieldDryRun      = "dry_run"
	logFieldForce       = "force"
	logFieldKeepDaily   = "keep_daily"
	logFieldKeepMonthly = "keep_monthly"
	logFieldKeepWeekly  = "keep_weekly"
	logFieldMaxAge      = "max_age"
	logFieldMaxCount    = "max_count"
	logFieldStorageURL  = "storage_url"
)

func Command() *cli.Command {
	fl := flags.Storage()
	fl = append(fl, flags.Retention()...)
	fl = append(fl,
		&cli.BoolFlag{
			Name:     "dry-run",
			Usage:    "show what would be deleted without actually deleting",
			Sources:  cli.EnvVars("RETENTION__DRY_RUN"),
			Category: "Retention",
		},
		&cli.BoolFlag{
			Name:     "force",
			Usage:    "continue even if errors occur",
			Sources:  cli.EnvVars("RETENTION__FORCE"),
			Category: "Retention",
		},
	)

	return &cli.Command{
		Name:    "retention",
		Aliases: []string{"ret"},
		Usage:   "Apply retention policies to backups",
		Flags:   fl,
		Action:  retentionAction,
	}
}

func retentionAction(ctx context.Context, cmd *cli.Command) error {
	logger := logging.GetLogger(ctx)
	if logger == nil {
		return cli.Exit("failed to retrieve logger", codes.InternalError)
	}

	operationID := logging.GenerateOperationID("retention")
	logger = logger.WithContext("retention-cmd", operationID)

	logger.Info(ctx, "Retention command initiated")

	// Log command parameters for debugging
	logger.Debug(ctx, "Parsing command parameters", logging.Fields{
		logFieldStorageURL:  cmd.String("storage-url"),
		logFieldMaxCount:    cmd.Int("retention-count"),
		logFieldMaxAge:      cmd.Duration("max-age"),
		logFieldKeepDaily:   cmd.Int("keep-daily"),
		logFieldKeepWeekly:  cmd.Int("keep-weekly"),
		logFieldKeepMonthly: cmd.Int("keep-monthly"),
		logFieldDryRun:      cmd.Bool("dry-run"),
		logFieldForce:       cmd.Bool("force"),
	})

	cfg, err := parseConfig(cmd)
	if err != nil {
		logger.Error(ctx, "Failed to parse configuration", err)
		return cli.Exit("retention command failed", codes.ParamsError)
	}

	logger.Debug(ctx, "Configuration validated successfully", logging.Fields{
		logFieldMaxCount:    cfg.MaxCount,
		logFieldMaxAge:      cfg.MaxAge.String(),
		logFieldKeepDaily:   cfg.KeepDaily,
		logFieldKeepWeekly:  cfg.KeepWeekly,
		logFieldKeepMonthly: cfg.KeepMonthly,
		logFieldDryRun:      cfg.DryRun,
		logFieldForce:       cfg.Force,
	})

	storageCfg := config.Storage{
		URL: cmd.String("storage-url"),
	}
	u, err := storageCfg.GetURL()
	if err != nil {
		logger.Error(ctx, "Failed to parse storage url", err)
		return cli.Exit("retention command failed", codes.ParamsError)
	}

	storageSvc, err := storage.New(u)
	if err != nil {
		logger.Error(ctx, "Failed to initialize storage backend", err)
		return cli.Exit("retention command failed", codes.InternalError)
	}
	defer func() {
		if closeErr := storageSvc.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to close storage backend", closeErr)
		}
	}()

	registrySvc := registry.NewService(storageSvc)

	logger.Info(ctx, "Starting retention execution", logging.Fields{
		logFieldStorageURL:  storageCfg.URL,
		logFieldMaxCount:    cfg.MaxCount,
		logFieldMaxAge:      cfg.MaxAge.String(),
		logFieldKeepDaily:   cfg.KeepDaily,
		logFieldKeepWeekly:  cfg.KeepWeekly,
		logFieldKeepMonthly: cfg.KeepMonthly,
		logFieldDryRun:      cfg.DryRun,
		logFieldForce:       cfg.Force,
	})

	if opErr := retention.NewOperation(cfg, registrySvc, storageSvc, logger).Run(ctx); opErr != nil {
		logger.Error(ctx, "Retention command failed", opErr)
		return cli.Exit("retention command failed", codes.InternalError)
	}

	logger.Info(ctx, "Retention command completed successfully")
	return nil
}
