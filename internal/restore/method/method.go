package method

import (
	"context"
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/restore/method/mariadb"
	"github.com/capcom6/mariadb-backup-s3/internal/restore/method/mongodb"
)

// Method runs the engine-specific restore step after the shared
// download/decrypt/extract pipeline has completed.
type Method interface {
	// Restore applies the files extracted into targetDir to the database.
	Restore(ctx context.Context, targetDir string, logger logging.Logger) error
}

// New selects the restore method for a database configuration. An empty
// engine is treated as MariaDB to preserve the historical extract-only
// behavior.
func New(db config.Database) (Method, error) {
	engine := db.Engine
	if engine == "" {
		engine = config.EngineMariaDB
	}

	switch engine {
	case config.EngineMariaDB:
		return mariadb.New(), nil
	case config.EngineMongoDB:
		return mongodb.New(mongodb.Config{
			Host:           db.Host,
			Port:           db.Port,
			User:           db.User,
			Password:       db.Password,
			RestoreOptions: db.RestoreOptions,
			RestoreBinary:  db.RestoreBinary,
			MongoDB:        db.MongoDB,
		}), nil
	default:
		return nil, fmt.Errorf("%w: %q", config.ErrInvalidEngine, engine)
	}
}
