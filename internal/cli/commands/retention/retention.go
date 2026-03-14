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
		"storage_url":  cmd.String("storage-url"),
		"max_count":    cmd.Int("retention-count"),
		"max_age":      cmd.Duration("max-age"),
		"keep_daily":   cmd.Int("keep-daily"),
		"keep_weekly":  cmd.Int("keep-weekly"),
		"keep_monthly": cmd.Int("keep-monthly"),
		"dry_run":      cmd.Bool("dry-run"),
		"force":        cmd.Bool("force"),
	})

	cfg, err := parseConfig(cmd)
	if err != nil {
		logger.Error(ctx, "Failed to parse configuration", err)
		return cli.Exit("retention command failed", codes.ParamsError)
	}

	logger.Debug(ctx, "Configuration validated successfully", logging.Fields{
		"max_count":    cfg.MaxCount,
		"max_age":      cfg.MaxAge.String(),
		"keep_daily":   cfg.KeepDaily,
		"keep_weekly":  cfg.KeepWeekly,
		"keep_monthly": cfg.KeepMonthly,
		"dry_run":      cfg.DryRun,
		"force":        cfg.Force,
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
		"storage_url":  storageCfg.URL,
		"max_count":    cfg.MaxCount,
		"max_age":      cfg.MaxAge.String(),
		"keep_daily":   cfg.KeepDaily,
		"keep_weekly":  cfg.KeepWeekly,
		"keep_monthly": cfg.KeepMonthly,
		"dry_run":      cfg.DryRun,
		"force":        cfg.Force,
	})

	if opErr := retention.NewOperation(cfg, registrySvc, storageSvc, logger).Run(ctx); opErr != nil {
		logger.Error(ctx, "Retention command failed", opErr)
		return cli.Exit("retention command failed", codes.InternalError)
	}

	logger.Info(ctx, "Retention command completed successfully")
	return nil
}
