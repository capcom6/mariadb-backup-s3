package logical

import (
	"context"
	"fmt"
	"io"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/sqlstream"
)

// ExtractStream splits a decompressed streamed logical backup into the
// dump of every database in targetDir.
func ExtractStream(ctx context.Context, r io.Reader, targetDir string, logger logging.Logger) error {
	if splitErr := sqlstream.SplitToDir(r, targetDir); splitErr != nil {
		logger.Error(ctx, "Failed to split the backup", splitErr, logging.Fields{})
		return fmt.Errorf("failed to split the backup: %w", splitErr)
	}

	return nil
}
