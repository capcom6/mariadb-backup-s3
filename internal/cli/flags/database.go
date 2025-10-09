package flags

import (
	"github.com/capcom6/mariadb-backup-s3/pkg/cliutil"
	"github.com/urfave/cli/v3"
)

func Database() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "db-host",
			Aliases:     []string{"host"},
			Usage:       "database host",
			DefaultText: "localhost",
			Sources:     cli.NewValueSourceChain(cli.EnvVar("MARIADB__HOST"), cliutil.DefaultValue("localhost")),
		},
		&cli.IntFlag{
			Name:        "db-port",
			Aliases:     []string{"port"},
			Usage:       "database port",
			DefaultText: "3306",
			Sources:     cli.NewValueSourceChain(cli.EnvVar("MARIADB__PORT"), cliutil.DefaultValue("3306")),
		},
		&cli.StringFlag{
			Name:        "db-user",
			Aliases:     []string{"user"},
			Usage:       "database user",
			DefaultText: "root",
			Sources:     cli.NewValueSourceChain(cli.EnvVar("MARIADB__USER"), cliutil.DefaultValue("root")),
		},
		&cli.StringFlag{
			Name:    "db-password",
			Aliases: []string{"password"},
			Usage:   "database password",
			Sources: cli.EnvVars("MARIADB__PASSWORD"),
		},
	}
}
