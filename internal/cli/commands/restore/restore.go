package restore

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
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
		Action: func(c context.Context, cmd *cli.Command) error {
			if cmd.StringArg("backup-file") == "" {
				return cli.Exit("backup file name is required", codes.ParamsError)
			}

			return restore.Execute(
				c,
				cmd.StringArg("backup-file"),
				config.Storage{
					URL: cmd.String("storage-url"),
				},
				config.Encryption{
					EncryptionKey: cmd.String("encryption-key"),
				},
				restore.Config{
					TargetDir: cmd.String("target-dir"),
				},
			)
		},
	}
}
