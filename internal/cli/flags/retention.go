package flags

import (
	"time"

	"github.com/urfave/cli/v3"
)

const (
	categoryRetention    = "Retention"
	defaultTextUnlimited = "unlimited"
)

func Retention() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "retention-count",
			Aliases:     []string{"backup-limits-max-count"},
			Usage:       "number of backups to retain (0 = unlimited)",
			Required:    false,
			DefaultText: defaultTextUnlimited,
			Sources:     cli.EnvVars("RETENTION__COUNT", "BACKUP__LIMITS__MAX_COUNT"),
			Category:    categoryRetention,
		},
		&cli.DurationFlag{
			Name:        "max-age",
			Usage:       "maximum age of backups to keep (e.g. 24h, 168h, 672h)",
			DefaultText: defaultTextUnlimited,
			Sources:     cli.EnvVars("RETENTION__MAX_AGE"),
			Category:    categoryRetention,
		},
		&cli.IntFlag{
			Name:        "keep-daily",
			Usage:       "number of daily backups to keep",
			DefaultText: defaultTextUnlimited,
			Sources:     cli.EnvVars("RETENTION__KEEP_DAILY"),
			Category:    categoryRetention,
		},
		&cli.IntFlag{
			Name:        "keep-weekly",
			Usage:       "number of weekly backups to keep",
			DefaultText: defaultTextUnlimited,
			Sources:     cli.EnvVars("RETENTION__KEEP_WEEKLY"),
			Category:    categoryRetention,
		},
		&cli.IntFlag{
			Name:        "keep-monthly",
			Usage:       "number of monthly backups to keep",
			DefaultText: defaultTextUnlimited,
			Sources:     cli.EnvVars("RETENTION__KEEP_MONTHLY"),
			Category:    categoryRetention,
		},
	}
}

type RetentionFlags struct {
	MaxCount    int
	MaxAge      time.Duration
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
}

func ParseRetentionFlags(cmd *cli.Command) RetentionFlags {
	return RetentionFlags{
		MaxCount:    cmd.Int("retention-count"),
		MaxAge:      cmd.Duration("max-age"),
		KeepDaily:   cmd.Int("keep-daily"),
		KeepWeekly:  cmd.Int("keep-weekly"),
		KeepMonthly: cmd.Int("keep-monthly"),
	}
}
