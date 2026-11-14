// Package artifact matches a backup artifact stream to the extracting
// method that produced it. It is the single owner of the artifact catalog:
// callers hand it a decompressed stream and the artifact filename and do not
// need to know which backup method emitted the artifact.
package artifact

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/method"
	"github.com/capcom6/mariadb-backup-s3/internal/method/logical"
	"github.com/capcom6/mariadb-backup-s3/internal/method/mongodb"
	"github.com/capcom6/mariadb-backup-s3/internal/sqlstream"
)

// Restore writes a decompressed backup stream into targetDir, dispatching on
// the artifact type encoded in filename: a streamed logical backup is split
// into one file per database, a streamed mongodump archive is written as a
// single archive file, and anything else is treated as a tar archive. The
// optional .enc suffix is stripped before matching.
func Restore(ctx context.Context, r io.Reader, targetDir string, filename string, logger logging.Logger) error {
	name := strings.TrimSuffix(filename, ".enc")

	var err error
	switch {
	case strings.HasSuffix(name, "."+sqlstream.ArtifactExtension):
		err = logical.ExtractStream(ctx, r, targetDir, logger)
	case strings.HasSuffix(name, "."+mongodb.StreamArtifactExtension):
		err = mongodb.ExtractStream(r, targetDir, filename)
	default:
		err = method.ExtractTar(ctx, r, targetDir, logger)
	}
	if err != nil {
		return fmt.Errorf("failed to extract %q: %w", filename, err)
	}

	return nil
}
