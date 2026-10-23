package mariadb

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

// Method is the MariaDB restore implementation. MariaDB restores stay
// extract-only, so Restore is a no-op that preserves that behavior.
type Method struct{}

// New creates the MariaDB restore method.
func New() *Method {
	return &Method{}
}

// Restore is a no-op: extraction into targetDir already completes a MariaDB
// restore, including for logical .sql backups.
func (*Method) Restore(_ context.Context, _ string, _ logging.Logger) error {
	return nil
}
