package backup

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	fl := flags.Database()
	fl = append(fl, flags.Storage()...)
	fl = append(fl, flags.Encryption()...)
	fl = append(fl,
		&cli.StringFlag{
			Name:    "db-backup-options",
			Usage:   "mariadb-backup additional options",
			Sources: cli.EnvVars("MARIADB__BACKUP_OPTIONS"),
		},
		&cli.IntFlag{
			Name:        "backup-limits-max-count",
			Usage:       "maximum number of backups to retain (0 = unlimited)",
			DefaultText: "0",
			Sources:     cli.EnvVars("BACKUP__LIMITS__MAX_COUNT"),
		},
	)

	return &cli.Command{
		Name:    "backup",
		Aliases: []string{"b"},
		Usage:   "Backup MariaDB database",
		Flags:   fl,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logger := logging.GetLogger(ctx)
			if logger == nil {
				return cli.Exit("failed to retrieve logger", codes.InternalError)
			}

			operationID := logging.GenerateOperationID("backup")
			logger = logger.WithContext("backup-cmd", operationID)

			logger.Info(ctx, "Backup command initiated")

			// Log command parameters for debugging
			logger.Debug(ctx, "Parsing command parameters", logging.Fields{
				"db_host":                 cmd.String("db-host"),
				"db_port":                 cmd.Int("db-port"),
				"db_user":                 cmd.String("db-user"),
				"db_password":             "***", // Don't log actual password
				"db_backup_options":       cmd.String("db-backup-options"),
				"backup_limits_max_count": cmd.Int("backup-limits-max-count"),
				"storage_url":             cmd.String("storage-url"),
				"encryption_key":          "***", // Don't log actual key
			})

			cfg := backup.DefaultConfig()

			// Configure MariaDB settings
			cfg.MariaDB.Host = cmd.String("db-host")
			cfg.MariaDB.Port = cmd.Int("db-port")
			cfg.MariaDB.User = cmd.String("db-user")
			cfg.MariaDB.Password = cmd.String("db-password")
			cfg.MariaDB.BackupOptions = cmd.String("db-backup-options")

			// Configure backup limits
			cfg.Backup.Limits.MaxCount = cmd.Int("backup-limits-max-count")

			// Configure storage
			cfg.Storage.URL = cmd.String("storage-url")

			// Configure encryption
			cfg.Encryption.EncryptionKey = cmd.String("encryption-key")

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				logger.Error(ctx, "Invalid configuration", err)
				return cli.Exit("backup command failed", codes.ParamsError)
			}
			logger.Debug(ctx, "Configuration validated successfully", logging.Fields{
				"db_host":                 cfg.MariaDB.Host,
				"db_port":                 cfg.MariaDB.Port,
				"db_user":                 cfg.MariaDB.User,
				"storage_url":             cfg.Storage.URL,
				"backup_limits_max_count": cfg.Backup.Limits.MaxCount,
				"encryption_enabled":      cfg.Encryption.Enabled(),
			})

			logger.Info(ctx, "Starting backup execution", logging.Fields{
				"db_host":                 cfg.MariaDB.Host,
				"db_port":                 cfg.MariaDB.Port,
				"backup_limits_max_count": cfg.Backup.Limits.MaxCount,
				"encryption_enabled":      cfg.Encryption.Enabled(),
			})

			if err := backup.NewOperation(cfg, logger).Run(ctx); err != nil {
				logger.Error(ctx, "Backup command failed", err)
				return cli.Exit("backup command failed", codes.InternalError)
			}

			logger.Info(ctx, "Backup command completed successfully")
			return nil
		},
	}
}
