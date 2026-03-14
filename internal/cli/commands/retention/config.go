package retention

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/retention"
	"github.com/urfave/cli/v3"
)

func parseConfig(cmd *cli.Command) (retention.Config, error) {
	f := flags.ParseRetentionFlags(cmd)

	// Parse retention configuration
	retentionCfg := retention.Config{
		MaxCount:    f.MaxCount,
		MaxAge:      f.MaxAge,
		KeepDaily:   f.KeepDaily,
		KeepWeekly:  f.KeepWeekly,
		KeepMonthly: f.KeepMonthly,
		DryRun:      cmd.Bool("dry-run"),
		Force:       cmd.Bool("force"),
	}

	// Validate configuration
	if err := retentionCfg.Validate(); err != nil {
		return retention.Config{}, fmt.Errorf("failed to parse retention configuration: %w", err)
	}

	return retentionCfg, nil
}
