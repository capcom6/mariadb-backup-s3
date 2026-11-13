package restore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/method/artifact"
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

// extract unpacks the decompressed backup stream into the target directory
// by delegating to the artifact's producing method. The dispatcher in
// [artifact.Restore] owns the artifact catalog; the restore pipeline only
// streams bytes.
func (o *Operation) extract(ctx context.Context, r io.Reader, targetDir string, filename string) error {
	o.logger.Info(ctx, "Stage 3: Extract")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 3 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	if extractErr := artifact.Restore(ctx, r, targetDir, filename, o.logger); extractErr != nil {
		return fmt.Errorf("failed to extract backup: %w", extractErr)
	}

	return nil
}
