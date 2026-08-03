package method

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

// Method defines the interface for backup tool implementations.
// Physical (mariadb-backup) and logical (mariadb-dump) methods both
// produce files in a temp directory that the caller compresses, encrypts,
// and uploads through a shared pipeline.
type Method interface {
	// Backup executes the backup tool, writing output files into tempdir.
	Backup(ctx context.Context, tempdir string, logger logging.Logger) error

	// Prepare runs any post-backup preparation step.
	// Physical backups require mariadb-backup --prepare; logical backups return nil.
	Prepare(ctx context.Context, tempdir string, logger logging.Logger) error

	// MethodName returns the identifier stored in registry metadata
	// (e.g. "mariadb-backup" or "mariadb-dump").
	MethodName() string
}
