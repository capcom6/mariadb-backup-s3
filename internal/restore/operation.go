package restore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
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
		func(ctx context.Context, r io.Reader, _ io.Writer) error {
			return o.extract(ctx, r, targetDir)
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

func (o *Operation) extract(ctx context.Context, r io.Reader, targetDir string) error {
	o.logger.Info(ctx, "Stage 2: Decompress")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 2 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	pigzCmd := exec.CommandContext(ctx, "pigz", "-d")
	tarCmd := exec.CommandContext(ctx, "tar", "-C", targetDir, "-xf", "-")

	// Set up piping
	var err error
	tarCmd.Stdin, err = pigzCmd.StdoutPipe()
	if err != nil {
		o.logger.Error(ctx, "Decompression failed: failed to create stdout pipe", err)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	pigzCmd.Stdin = r

	var tarErr, pigzErr bytes.Buffer
	var errs []error
	tarCmd.Stderr = &tarErr
	pigzCmd.Stderr = &pigzErr

	if pigzErr := pigzCmd.Start(); pigzErr != nil {
		o.logger.Error(ctx, "Decompression failed: failed to start pigz", pigzErr)
		return fmt.Errorf("failed to start pigz: %w", pigzErr)
	}
	if tarErr := tarCmd.Run(); tarErr != nil {
		o.logger.Error(ctx, "Decompression failed: failed to run tar", tarErr)
		errs = append(errs, fmt.Errorf("failed to run tar: %w", tarErr))
	}
	if waitErr := pigzCmd.Wait(); waitErr != nil {
		o.logger.Error(ctx, "Decompression failed: failed to wait for pigz", waitErr)
		errs = append(errs, fmt.Errorf("failed to wait for pigz: %w", waitErr))
	}

	// Attach stderr as context only if we observed command errors above.
	if len(errs) > 0 {
		if s := tarErr.String(); s != "" {
			errs = append(errs, fmt.Errorf("%w: tar stderr: %s", ErrExternalCommandFailed, s))
		}
		if s := pigzErr.String(); s != "" {
			errs = append(errs, fmt.Errorf("%w: pigz stderr: %s", ErrExternalCommandFailed, s))
		}
	}

	return errors.Join(errs...)
}
