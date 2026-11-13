package mongodb

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

// ExtractStream writes a decompressed streamed mongodump backup into
// targetDir as a single file, named after the artifact without its
// .gz[.enc] suffixes. Importing it into MongoDB is a separate mongorestore
// step.
func ExtractStream(
	ctx context.Context,
	r io.Reader,
	targetDir string,
	filename string,
	logger logging.Logger,
) (err error) {
	// The artifact name comes from the registry or a CLI argument; keep only
	// its base name so a crafted name cannot escape the target directory.
	outName := filepath.Base(strings.TrimSuffix(strings.TrimSuffix(filename, ".enc"), ".gz"))
	outFile, err := os.Create(filepath.Join(targetDir, outName))
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer func() {
		if closeErr := outFile.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to close archive file", closeErr, logging.Fields{})
			if err == nil {
				err = fmt.Errorf("failed to close archive file: %w", closeErr)
			}
		}
	}()

	if _, copyErr := io.Copy(outFile, r); copyErr != nil {
		return fmt.Errorf("failed to write archive file: %w", copyErr)
	}

	return nil
}
