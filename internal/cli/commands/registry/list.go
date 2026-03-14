package registry

import (
	"context"
	"errors"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/core/codes"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/urfave/cli/v3"
)

func ListCommand() *cli.Command {
	fl := flags.Storage()
	fl = append(fl, &cli.BoolFlag{
		Name:    "rebuild",
		Aliases: []string{"r"},
		Usage:   "Rebuild registry from storage if missing",
		Value:   false,
	})

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List backups from registry (read-only by default, use --rebuild to rebuild if missing)",
		Flags:   fl,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logger := logging.GetLogger(ctx)
			if logger == nil {
				return cli.Exit("failed to retrieve logger", codes.InternalError)
			}

			u, err := (config.Storage{URL: cmd.String("storage-url")}).GetURL()
			if err != nil {
				logger.Error(ctx, "Failed to parse storage URL", err)
				return cli.Exit("invalid storage url", codes.ParamsError)
			}

			storageSvc, err := storage.New(u)
			if err != nil {
				logger.Error(ctx, "Failed to initialize storage backend", err)
				return cli.Exit("failed to initialize storage backend", codes.InternalError)
			}
			defer func() {
				if closeErr := storageSvc.Close(); closeErr != nil {
					logger.Error(ctx, "Failed to close storage backend", closeErr)
				}
			}()

			// Conditionally enable recovery based on --rebuild flag
			var opts []registry.Option
			if cmd.Bool("rebuild") {
				opts = append(opts, registry.WithRecovery())
			}
			registrySvc := registry.NewService(storageSvc, opts...)

			reg, err := registrySvc.Load(ctx)
			if err != nil {
				// If registry not found and not rebuilding, return empty list
				if errors.Is(err, storage.ErrNotFound) && !cmd.Bool("rebuild") {
					logger.Warn(ctx, "Registry not found, returning empty list", nil)
					printRegistryTable(ctx, logger, registry.New())
					return nil
				}
				logger.Error(ctx, "Failed to load registry", err)
				return cli.Exit("failed to load registry", codes.InternalError)
			}

			reg.SortByCreatedAtDesc()
			printRegistryTable(ctx, logger, reg)

			return nil
		},
	}
}

func printRegistryTable(ctx context.Context, logger logging.Logger, reg *registry.Registry) {
	logger.Info(ctx, "Listing backups", logging.Fields{
		"updated_at": reg.UpdatedAt,
		"version":    reg.Version,
		"count":      len(reg.Backups),
	})

	for _, v := range reg.Backups {
		logger.Info(ctx, "Backup", logging.Fields{
			"id":         v.ID,
			"created_at": v.CreatedAt,
			"status":     v.Status,
			"encrypted":  v.Encrypted,
			"size":       v.SizeBytes,
			"filename":   v.Filename,
		})
	}
}
