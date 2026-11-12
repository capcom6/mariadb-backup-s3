package restore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/mongodb"
	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/sqlstream"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
)

const (
	logFieldDuration = "duration"
	logFieldFilename = "filename"
)

type Operation struct {
	config Config

	logger logging.Logger
}

func NewOperation(config Config, logger logging.Logger) *Operation {
	return &Operation{
		config: config,
		logger: logger,
	}
}

func (o *Operation) Run(ctx context.Context, filename string) error {
	ctx = logging.WithComponent(ctx, "restore")

	o.logger.Info(ctx, "Starting restore")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Restore completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	targetDir := o.config.TargetDir
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	err := pipeline.Run(
		ctx,
		nil,
		nil,
		func(ctx context.Context, _ io.Reader, w io.Writer) error {
			return o.download(ctx, filename, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return o.decrypt(ctx, r, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return o.gunzip(ctx, r, w)
		},
		func(ctx context.Context, r io.Reader, _ io.Writer) error {
			return o.extract(ctx, r, targetDir, filename)
		},
	)

	if err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}

	return nil
}

func (o *Operation) download(ctx context.Context, filename string, w io.Writer) error {
	o.logger.Info(ctx, "Stage 1: Download")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 1 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	u, err := o.config.Storage.GetURL()
	if err != nil {
		o.logger.Error(ctx, "Download failed: failed to parse storage url", err, logging.Fields{
			logFieldFilename: filename,
		})
		return fmt.Errorf("failed to parse storage url: %w", err)
	}

	storageBackend, err := storage.New(u)
	if err != nil {
		o.logger.Error(ctx, "Download failed: failed to create storage backend", err)
		return fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Download the backup
	if downErr := storageBackend.Download(ctx, filename, w); downErr != nil {
		o.logger.Error(ctx, "Download failed", downErr, logging.Fields{
			logFieldFilename: filename,
		})
		return fmt.Errorf("failed to download: %w", downErr)
	}

	return nil
}

func (o *Operation) decrypt(ctx context.Context, r io.Reader, w io.Writer) error {
	if !o.config.Encryption.Enabled() {
		_, err := io.Copy(w, r)
		if err != nil {
			o.logger.Error(ctx, "Failed to copy data", err)
			return fmt.Errorf("failed to copy data: %w", err)
		}
		return nil
	}

	o.logger.Info(ctx, "Stage 1.1: Decrypt")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 1.1 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	masterKey, err := o.config.Encryption.Key()
	if err != nil {
		o.logger.Error(ctx, "Failed to get key", err)
		return fmt.Errorf("failed to get key: %w", err)
	}

	service := encryption.NewAES256GCMService(masterKey)
	if decErr := service.Decrypt(ctx, r, w); decErr != nil {
		o.logger.Error(ctx, "Failed to decrypt", decErr)
		return fmt.Errorf("failed to decrypt: %w", decErr)
	}

	return nil
}

// gunzip decompresses the artifact stream with pigz. Every artifact type is
// gzipped, and pigz is already a hard dependency (the backup side compresses
// with it), so decompression runs once here with the same parallelism for
// every format.
func (o *Operation) gunzip(ctx context.Context, r io.Reader, w io.Writer) error {
	o.logger.Info(ctx, "Stage 2: Decompress")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 2 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	pigzCmd := exec.CommandContext(ctx, "pigz", "-d")
	pigzCmd.Stdin = r
	pigzCmd.Stdout = w

	var stderr bytes.Buffer
	pigzCmd.Stderr = &stderr

	if runErr := pigzCmd.Run(); runErr != nil {
		o.logger.Error(ctx, "Decompression failed", runErr, logging.Fields{})
		var errs []error
		errs = append(errs, fmt.Errorf("failed to decompress: %w", runErr))
		if s := strings.TrimSpace(stderr.String()); s != "" {
			errs = append(errs, fmt.Errorf("%w: pigz stderr: %s", ErrExternalCommandFailed, s))
		}
		return errors.Join(errs...)
	}

	return nil
}

// extract unpacks the decompressed backup stream into the target directory,
// dispatching on the artifact type. A streamed logical backup is split into
// one file per database, a streamed mongodump archive is written as a single
// archive file, an archive is extracted.
func (o *Operation) extract(ctx context.Context, r io.Reader, targetDir string, filename string) error {
	o.logger.Info(ctx, "Stage 3: Extract")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 3 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	switch {
	case isSQLStream(filename):
		return o.extractSQLStream(ctx, r, targetDir)
	case isArchiveStream(filename):
		return o.extractArchiveStream(ctx, r, targetDir, filename)
	default:
		return o.extractArchive(ctx, r, targetDir)
	}
}

// isSQLStream reports whether the artifact holds a streamed logical backup.
func isSQLStream(filename string) bool {
	name := strings.TrimSuffix(filename, ".enc")

	return strings.HasSuffix(name, "."+sqlstream.ArtifactExtension)
}

// isArchiveStream reports whether the artifact holds a streamed mongodump
// archive.
func isArchiveStream(filename string) bool {
	name := strings.TrimSuffix(filename, ".enc")

	return strings.HasSuffix(name, "."+mongodb.StreamArtifactExtension)
}

// extractSQLStream splits a decompressed streamed logical backup into the
// dump of every database.
func (o *Operation) extractSQLStream(ctx context.Context, r io.Reader, targetDir string) error {
	if splitErr := sqlstream.SplitToDir(r, targetDir); splitErr != nil {
		o.logger.Error(ctx, "Failed to split the backup", splitErr, logging.Fields{})
		return fmt.Errorf("failed to split the backup: %w", splitErr)
	}

	return nil
}

// extractArchiveStream writes a decompressed streamed mongodump backup into
// the target directory as a single file, named after the artifact without its
// .gz[.enc] suffixes. Importing it into MongoDB is a separate mongorestore
// step.
func (o *Operation) extractArchiveStream(
	ctx context.Context,
	r io.Reader,
	targetDir string,
	filename string,
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
			o.logger.Error(ctx, "Failed to close archive file", closeErr, logging.Fields{})
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

// extractArchive extracts a decompressed tar archive into the target
// directory.
func (o *Operation) extractArchive(ctx context.Context, r io.Reader, targetDir string) error {
	tarCmd := exec.CommandContext(ctx, "tar", "-C", targetDir, "-xf", "-")
	tarCmd.Stdin = r

	var stderr bytes.Buffer
	tarCmd.Stderr = &stderr

	var errs []error
	if runErr := tarCmd.Run(); runErr != nil {
		o.logger.Error(ctx, "Failed to run tar", runErr, logging.Fields{})
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
