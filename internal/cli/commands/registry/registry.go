package registry

import (
	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	fl := flags.Storage()

	return &cli.Command{
		Name:    "registry",
		Aliases: []string{"reg"},
		Usage:   "Manage registry",
		Flags:   fl,
		Commands: []*cli.Command{
			ListCommand(),
		},
	}
}
