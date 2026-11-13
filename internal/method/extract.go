package method

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

// ErrExternalCommandFailed indicates that an external helper command exited
// with a failure while extracting an artifact.
var ErrExternalCommandFailed = errors.New("external command failed")

// ExtractTar extracts a decompressed tar archive stream into targetDir.
// File-mode artifacts (physical and logical backups) ship as a tar archive,
// so this is the shared extractor for every artifact named [ArtifactTarGz].
func ExtractTar(ctx context.Context, r io.Reader, targetDir string, logger logging.Logger) error {
	tarCmd := exec.CommandContext(ctx, "tar", "-C", targetDir, "-xf", "-")
	tarCmd.Stdin = r

	var stderr bytes.Buffer
	tarCmd.Stderr = &stderr

	var errs []error
	if runErr := tarCmd.Run(); runErr != nil {
		logger.Error(ctx, "Failed to run tar", runErr, logging.Fields{})
		errs = append(errs, fmt.Errorf("failed to run tar: %w", runErr))
	}

	// Attach stderr as context only if we observed a command error above.
	if len(errs) > 0 {
		if s := stderr.String(); s != "" {
			errs = append(errs, fmt.Errorf("%w: tar stderr: %s", ErrExternalCommandFailed, s))
		}
	}

	return errors.Join(errs...)
}
