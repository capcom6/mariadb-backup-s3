package backup

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	fl := flags.Database()
	fl = append(fl, flags.Encryption()...)
	fl = append(fl, flags.Storage()...)
	fl = append(fl,
		&cli.StringFlag{
			Name:        "db-backup-options",
			Usage:       "database backup options",
			DefaultText: "mariadb-backup additional options",
			Sources:     cli.EnvVars("MARIADB__BACKUP_OPTIONS"),
		},
		&cli.IntFlag{
			Name:        "backup-limits-max-count",
			Usage:       "maximum number of backups to retain (0 = unlimited)",
			DefaultText: "0",
			Sources:     cli.EnvVars("BACKUP__LIMITS__MAX_COUNT"),
		},
	)

	return &cli.Command{
		Name:  "backup",
		Usage: "Backup MariaDB database",
		Flags: fl,
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
