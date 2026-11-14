package mongodb

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractStream writes a decompressed streamed mongodump backup into
// targetDir as a single owner-only file, named after the artifact without
// its .gz[.enc] suffixes. Importing it into MongoDB is a separate mongorestore
// step.
func ExtractStream(
	r io.Reader,
	targetDir string,
	filename string,
) error {
	// The artifact name comes from the registry or a CLI argument; keep only
	// its base name so a crafted name cannot escape the target directory.
	outName := filepath.Base(strings.TrimSuffix(strings.TrimSuffix(filename, ".enc"), ".gz"))
	dest := filepath.Join(targetDir, outName)

	// Write to a uniquely named temporary file inside the trusted target
	// directory, then rename it over the destination. A pre-existing symlink
	// at the destination is replaced, not followed, so the archive cannot
	// truncate a file outside the directory. The temporary file is created
	// with owner-only permissions.
	outFile, err := os.CreateTemp(targetDir, "."+outName+".tmp")
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}

	if _, copyErr := io.Copy(outFile, r); copyErr != nil {
		_ = outFile.Close()
		_ = os.Remove(outFile.Name())
		return fmt.Errorf("failed to write archive file: %w", copyErr)
	}

	if closeErr := outFile.Close(); closeErr != nil {
		_ = os.Remove(outFile.Name())
		return fmt.Errorf("failed to close archive file: %w", closeErr)
	}

	if renameErr := os.Rename(outFile.Name(), dest); renameErr != nil {
		_ = os.Remove(outFile.Name())
		return fmt.Errorf("failed to finalize archive file: %w", renameErr)
	}

	return nil
}
