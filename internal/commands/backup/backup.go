package backup

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/pkg/cliutil"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:  "backup",
		Usage: "Backup MariaDB database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "storage-url",
				Aliases:  []string{"storage"},
				Usage:    "Storage URL (e.g., s3://bucket/path, file:///path, ftp://host/path)",
				Required: true,
				Sources:  cli.EnvVars("STORAGE__URL"),
			},
			&cli.StringFlag{
				Name:        "db-host",
				Aliases:     []string{"host"},
				Usage:       "Database Host",
				DefaultText: "localhost",
				Sources:     cli.NewValueSourceChain(cli.EnvVar("MARIADB__HOST"), cliutil.DefaultValue("localhost")),
			},
			&cli.IntFlag{
				Name:        "db-port",
				Aliases:     []string{"port"},
				Usage:       "Database Port",
				DefaultText: "3306",
				Sources:     cli.NewValueSourceChain(cli.EnvVar("MARIADB__PORT"), cliutil.DefaultValue("3306")),
			},
			&cli.StringFlag{
				Name:        "db-user",
				Aliases:     []string{"user"},
				Usage:       "Database User",
				DefaultText: "root",
				Sources:     cli.NewValueSourceChain(cli.EnvVar("MARIADB__USER"), cliutil.DefaultValue("root")),
			},
			&cli.StringFlag{
				Name:    "db-password",
				Aliases: []string{"password"},
				Usage:   "Database Password",
				Sources: cli.EnvVars("MARIADB__PASSWORD"),
			},
			&cli.StringFlag{
				Name:        "db-backup-options",
				Usage:       "Database Backup Options",
				DefaultText: "",
				Sources:     cli.EnvVars("MARIADB__BACKUP_OPTIONS"),
			},
			&cli.IntFlag{
				Name:        "backup-limits-max-count",
				Usage:       "Maximum number of backups to retain (0 = unlimited)",
				DefaultText: "0",
				Sources:     cli.EnvVars("BACKUP__LIMITS__MAX_COUNT"),
			},
			&cli.StringFlag{
				Name:    "encryption-key",
				Usage:   "Encryption key (32 bytes, base64 encoded)",
				Sources: cli.EnvVars("ENCRYPTION__KEY"),
			},
		},
		Action: func(c context.Context, cmd *cli.Command) error {
			cfg := backup.DefaultConfig()

			cfg.MariaDB.Host = cmd.String("db-host")
			cfg.MariaDB.Port = cmd.Int("db-port")
			cfg.MariaDB.User = cmd.String("db-user")
			cfg.MariaDB.Password = cmd.String("db-password")
			cfg.MariaDB.BackupOptions = cmd.String("db-backup-options")

			cfg.Backup.Limits.MaxCount = cmd.Int("backup-limits-max-count")

			cfg.Storage.URL = cmd.String("storage-url")

			cfg.Encryption.EncryptionKey = cmd.String("encryption-key")

			return backup.Execute(c, cfg)
		},
	}
}
