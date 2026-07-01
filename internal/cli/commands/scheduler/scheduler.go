package scheduler

import (
	"context"
	"fmt"
	"os"

	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
	"github.com/urfave/cli/v3"
)

const (
	flagConfig    = "config"
	flagStateFile = "state-file"

	configUsage = "Path to scheduler YAML configuration"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:    "scheduler",
		Aliases: []string{"sched"},
		Usage:   "Run the backup scheduler daemon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     flagConfig,
				Aliases:  []string{"c"},
				Usage:    configUsage,
				Sources:  cli.EnvVars("SCHEDULER__CONFIG"),
				Required: true,
			},
		},
		DefaultCommand: "run",
		Commands: []*cli.Command{
			runCommand(),
			statusCommand(),
			checkCommand(),
		},
	}
}

func runCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Start the scheduler daemon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    flagStateFile,
				Usage:   "Path to scheduler state file",
				Sources: cli.EnvVars("SCHEDULER__STATE_FILE"),
			},
			&cli.BoolFlag{
				Name:  "once",
				Usage: "Run all jobs once and exit (for testing)",
			},
		},
		Action: runAction,
	}
}

func runAction(ctx context.Context, cmd *cli.Command) error {
	logger := logging.GetLogger(ctx)
	if logger == nil {
		return cli.Exit("failed to retrieve logger", codes.InternalError)
	}

	configPath := cmd.String(flagConfig)
	statePath := cmd.String(flagStateFile)

	sched, schedErr := scheduler.New(scheduler.Config{ConfigPath: configPath, StatePath: statePath}, logger)
	if schedErr != nil {
		logger.Error(ctx, "Failed to create scheduler", schedErr)
		return cli.Exit(fmt.Sprintf("failed to create scheduler: %v", schedErr), codes.InternalError)
	}

	if cmd.Bool("once") {
		logger.Info(ctx, "Running jobs once")
		runErr := sched.RunOnce(ctx)
		if closeErr := sched.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to save state", closeErr)
		}
		if runErr != nil {
			return cli.Exit(fmt.Sprintf("scheduler run once failed: %v", runErr), codes.InternalError)
		}
		return nil
	}

	logger.Info(ctx, "Starting scheduler daemon")

	if startErr := sched.Run(ctx); startErr != nil {
		return cli.Exit(fmt.Sprintf("scheduler failed: %v", startErr), codes.InternalError)
	}
	return nil
}

func statusCommand() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Aliases: []string{"st"},
		Usage:   "Show scheduler job statuses",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    flagStateFile,
				Usage:   "Path to scheduler state file",
				Sources: cli.EnvVars("SCHEDULER__STATE_FILE"),
			},
		},
		Action: statusAction,
	}
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	logger := logging.GetLogger(ctx)
	if logger == nil {
		return cli.Exit("failed to retrieve logger", codes.InternalError)
	}

	configPath := cmd.String(flagConfig)
	statePath := cmd.String(flagStateFile)

	sched, schedErr := scheduler.New(scheduler.Config{ConfigPath: configPath, StatePath: statePath}, logger)
	if schedErr != nil {
		logger.Error(ctx, "Failed to create scheduler", schedErr)
		return cli.Exit(fmt.Sprintf("failed to create scheduler: %v", schedErr), codes.InternalError)
	}
	defer func() {
		if err := sched.Close(); err != nil {
			logger.Error(ctx, "Failed to save state", err)
		}
	}()

	states := sched.Status()
	if len(states) == 0 {
		logger.Info(ctx, "No job state found")
		return nil
	}

	logger.Info(ctx, "Scheduler job statuses", logging.Fields{
		"jobs": len(states),
	})

	for _, st := range states {
		logger.Info(ctx, "Job status", logging.Fields{
			"name":          st.Name,
			"last_run":      st.LastRunAt.Format("2006-01-02 15:04:05"),
			"status":        string(st.LastRunStatus),
			"duration":      st.LastDuration,
			"next_run":      st.NextRunAt.Format("2006-01-02 15:04:05"),
			"run_count":     st.RunCount,
			"failure_count": st.FailureCount,
			"last_error":    st.LastRunError,
		})
	}

	return nil
}

func checkCommand() *cli.Command {
	return &cli.Command{
		Name:    "check",
		Aliases: []string{"validate"},
		Usage:   "Validate a scheduler configuration file",
		Action:  checkAction,
	}
}

func checkAction(ctx context.Context, cmd *cli.Command) error {
	logger := logging.GetLogger(ctx)
	if logger == nil {
		return cli.Exit("failed to retrieve logger", codes.InternalError)
	}

	configPath := cmd.String(flagConfig)
	cfg, err := repository.Load(configPath)
	if err != nil {
		logger.Error(ctx, "Configuration validation failed", err)
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)

		return cli.Exit("configuration is invalid", codes.ParamsError)
	}

	logger.Info(ctx, "Configuration is valid", logging.Fields{
		"config": configPath,
		"jobs":   len(cfg.Jobs),
	})

	for _, job := range cfg.Jobs {
		logger.Info(ctx, "Job", logging.Fields{
			"name":     job.Name,
			"command":  job.Command,
			"schedule": job.Schedule,
			"enabled":  job.IsEnabled(),
			"storage":  job.Storage.URL,
		})
	}

	return nil
}
