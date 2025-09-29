package main

import (
	"context"
	"log"
	"os"

	"github.com/capcom6/mariadb-backup-s3/internal/commands/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("failed to load .env: %s", err)
	}

	app := &cli.Command{
		Name:           "mariadb-backup-s3",
		Usage:          "🔁 Automated MariaDB database backups with S3-compatible storage integration",
		Version:        "dev",
		Description:    "Open-source solution for reliable, cloud-native MariaDB database backups with S3-compatible storage integration. 🛡️\nEasy setup with environment variables and command-line flags. ☁️",
		DefaultCommand: "backup",
		Commands: []*cli.Command{
			backup.Command(),
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "storage-url",
				Aliases:  []string{"storage"},
				Usage:    "Storage URL (e.g., s3://bucket/path, file:///path, ftp://host/path)",
				Required: true,
				Sources:  cli.EnvVars("STORAGE__URL"),
			},
		},
		Authors: []any{
			"Aleksandr Soloshenko <i@capcom.me>",
		},
		Copyright: "License: Apache-2.0",
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Printf("failed to run: %s", err)
		os.Exit(codes.InternalError)
	}
}
