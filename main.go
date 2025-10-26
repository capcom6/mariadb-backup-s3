package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/commands/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/cli/commands/restore"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"
)

//nolint:gochecknoglobals // build metadata
var (
	appVersion   = "dev"
	appBuildDate = "unknown"
	appGitCommit = "unknown"
	appGoVersion = runtime.Version()
)

func main() {
	logger := logging.NewDefault()
	defer func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close logger: %v\n", err)
		}
	}()

	ctx := logging.WithLogger(context.Background(), logger)

	// Load environment variables
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		logger.Fatal(ctx, "failed to load .env", err)
	}

	//nolint:reassign // urfave/cli specific
	cli.VersionPrinter = func(cmd *cli.Command) {
		fmt.Fprintf(cmd.Root().Writer, "Version:    %s\n", appVersion)
		fmt.Fprintf(cmd.Root().Writer, "Build Date: %s\n", appBuildDate)
		fmt.Fprintf(cmd.Root().Writer, "Git Commit: %s\n", appGitCommit)
		fmt.Fprintf(cmd.Root().Writer, "Go Version: %s\n", appGoVersion)
	}
	//nolint:reassign // urfave/cli specific
	cli.VersionFlag = &cli.BoolFlag{
		Name:        "version",
		Usage:       "print the version",
		HideDefault: true,
		Local:       true,
	}

	app := &cli.Command{
		Name:           "mariadb-backup-s3",
		Usage:          "🔁 Automated MariaDB database backups with S3-compatible storage integration",
		Version:        appVersion,
		Description:    "Open-source solution for reliable, cloud-native MariaDB database backups with S3-compatible storage integration. 🛡️\nEasy setup with environment variables and command-line flags. ☁️",
		DefaultCommand: "backup",
		Commands: []*cli.Command{
			backup.Command(),
			restore.Command(),
		},
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			logger.Info(ctx, "Starting MariaDB Backup S3 application")

			return ctx, nil
		},
		After: func(ctx context.Context, _ *cli.Command) error {
			logger.Info(ctx, "Application completed successfully")

			return nil
		},
		Flags: []cli.Flag{},
		Authors: []any{
			"Aleksandr Soloshenko <i@capcom.me>",
		},
		Copyright: "License: Apache-2.0",
	}

	if err := app.Run(ctx, os.Args); err != nil {
		logger.Error(ctx, "Application failed", err)
		os.Exit(codes.InternalError)
	}
}
