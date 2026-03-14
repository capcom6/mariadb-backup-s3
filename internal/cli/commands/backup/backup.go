package backup

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/retention"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/cliutil"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	fl := flags.Database()
	fl = append(fl, flags.Storage()...)
	fl = append(fl, flags.Encryption()...)
	fl = append(fl, flags.Retention()...)
	fl = append(fl,
		&cli.StringFlag{
			Name:    "db-backup-options",
			Usage:   "mariadb-backup additional options",
			Sources: cli.EnvVars("MARIADB__BACKUP_OPTIONS"),
		},
		&cli.StringFlag{
			Name:        "db-backup-binary",
			Usage:       "mariadb-backup binary path",
			DefaultText: "mariadb-backup",
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("MARIADB__BACKUP_BINARY"),
				cliutil.DefaultValue("mariadb-backup"),
			),
		},

		&cli.BoolFlag{
			Name:    "skip-retention",
			Usage:   "skip retention policy",
			Sources: cli.EnvVars("BACKUP__SKIP_RETENTION"),
		},
	)

	return &cli.Command{
		Name:    "backup",
		Aliases: []string{"b"},
		Usage:   "Backup MariaDB database",
		Flags:   fl,
		Commands: []*cli.Command{
			ListCommand(),
		},
		Action: backupAction,
	}
}

func backupAction(ctx context.Context, cmd *cli.Command) error {
	logger := logging.GetLogger(ctx)
	if logger == nil {
		return cli.Exit("failed to retrieve logger", codes.InternalError)
	}

	operationID := logging.GenerateOperationID("backup")
	logger = logger.WithContext("backup-cmd", operationID)

	logger.Info(ctx, "Backup command initiated")

	// Log command parameters for debugging
	logger.Debug(ctx, "Parsing command parameters", logging.Fields{
		"db_host":           cmd.String("db-host"),
		"db_port":           cmd.Int("db-port"),
		"db_user":           cmd.String("db-user"),
		"db_password":       "***", // Don't log actual password
		"db_backup_options": cmd.String("db-backup-options"),
		"db_backup_binary":  cmd.String("db-backup-binary"),
		"storage_url":       cmd.String("storage-url"),
		"encryption_key":    "***", // Don't log actual key
	})

	cfg, err := parseConfig(cmd)
	if err != nil {
		logger.Error(ctx, "Failed to parse configuration", err)
		return cli.Exit("backup command failed", codes.ParamsError)
	}

	logger.Debug(ctx, "Configuration validated successfully", logging.Fields{
		"db_host":            cfg.MariaDB.Host,
		"db_port":            cfg.MariaDB.Port,
		"db_user":            cfg.MariaDB.User,
		"storage_url":        cfg.Storage.URL,
		"encryption_enabled": cfg.Encryption.Enabled(),
	})

	u, err := cfg.Storage.GetURL()
	if err != nil {
		logger.Error(ctx, "Failed to parse storage url", err)
		return cli.Exit("backup command failed", codes.ParamsError)
	}

	storageSvc, err := storage.New(u)
	if err != nil {
		logger.Error(ctx, "Failed to initialize storage backend", err)
		return cli.Exit("backup command failed", codes.InternalError)
	}
	defer func() {
		if closeErr := storageSvc.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to close storage backend", closeErr)
		}
	}()

	registrySvc := registry.NewService(storageSvc)

	logger.Info(ctx, "Starting backup execution", logging.Fields{
		"db_host":            cfg.MariaDB.Host,
		"db_port":            cfg.MariaDB.Port,
		"encryption_enabled": cfg.Encryption.Enabled(),
	})

	if opErr := backup.NewOperation(cfg, registrySvc, storageSvc, logger).Run(ctx); opErr != nil {
		logger.Error(ctx, "Backup command failed", opErr)
		return cli.Exit("backup command failed", codes.InternalError)
	}

	if !cmd.Bool("skip-retention") {
		rflags := flags.ParseRetentionFlags(cmd)
		rcfg := retention.Config{
			MaxCount:    rflags.MaxCount,
			MaxAge:      rflags.MaxAge,
			KeepDaily:   rflags.KeepDaily,
			KeepWeekly:  rflags.KeepWeekly,
			KeepMonthly: rflags.KeepMonthly,
			DryRun:      false,
			Force:       false,
		}

		// Skip retention if no policies are configured
		if retErr := rcfg.Validate(); retErr != nil {
			logger.Warn(ctx, "Skipping retention: "+retErr.Error())
		} else if opErr := retention.NewOperation(rcfg, registrySvc, storageSvc, logger).Run(ctx); opErr != nil {
			logger.Error(ctx, "Retention command failed", opErr)
			return cli.Exit("retention command failed", codes.InternalError)
		}
	}

	logger.Info(ctx, "Backup command completed successfully")
	return nil
}
