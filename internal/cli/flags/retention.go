package flags

import "github.com/urfave/cli/v3"

func Retention() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "retention-count",
			Aliases:     []string{"backup-limits-max-count"},
			Usage:       "number of backups to retain (0 = unlimited)",
			Required:    false,
			DefaultText: "unlimited",
			Sources:     cli.EnvVars("RETENTION__COUNT", "BACKUP__LIMITS__MAX_COUNT"),
			Category:    "Retention",
		},
	}
}
