package backup

import (
	"context"

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

	return &cli.Command{
		Name:  "list",
		Usage: "List backups from registry",
		Flags: fl,
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
			backend, err := storage.New(u)
			if err != nil {
				logger.Error(ctx, "Failed to initialize storage backend", err)
				return cli.Exit("failed to initialize storage backend", codes.InternalError)
			}

			registrySvc := registry.NewService(backend, registry.WithRecovery())

			reg, err := registrySvc.Load(ctx)
			if err != nil {
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
