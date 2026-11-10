package method

import (
	"context"
	"io"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

// ArtifactTarGz is the extension of a gzipped tar archive, without the
// leading dot.
const ArtifactTarGz = "tar.gz"

// Method defines the interface for backup tool implementations.
// Physical (mariadb-backup) and logical (mariadb-dump) methods produce
// files in a temp directory that the caller compresses, encrypts,
// and uploads through a shared pipeline. Methods that can also emit their
// output as a stream additionally implement [StreamingMethod].
type Method interface {
	// Backup executes the backup tool, writing output files into tempdir.
	Backup(ctx context.Context, tempdir string, logger logging.Logger) error

	// Prepare runs any post-backup preparation step.
	// Physical backups require mariadb-backup --prepare; logical backups return nil.
	Prepare(ctx context.Context, tempdir string, logger logging.Logger) error

	// MethodName returns the identifier stored in registry metadata
	// (e.g. "mariadb-backup" or "mariadb-dump").
	MethodName() string

	// ArtifactExtension returns the extension of the artifact produced for
	// this method's backup, without the leading dot (e.g. "tar.gz").
	ArtifactExtension() string
}

// StreamingMethod is implemented by backup methods that can emit their output
// as a stream, instead of writing files into a temp directory.
type StreamingMethod interface {
	Method

	// Stream writes the backup content to w.
	Stream(ctx context.Context, w io.Writer, logger logging.Logger) error
}
