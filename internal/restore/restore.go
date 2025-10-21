package restore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log" //nolint:depguard // temporary
	"os"
	"os/exec"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
)

func Execute(
	ctx context.Context,
	filename string,
	storage config.Storage,
	encryption config.Encryption,
	config Config,
) error {
	log.Println("Starting restore")
	start := time.Now()
	defer func() {
		log.Printf("Restore completed in %v", time.Since(start))
	}()

	targetDir := config.TargetDir
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	err := pipeline.Run(
		ctx,
		nil,
		nil,
		func(ctx context.Context, _ io.Reader, w io.Writer) error {
			return download(ctx, storage, filename, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return decrypt(ctx, encryption, r, w)
		},
		func(ctx context.Context, r io.Reader, _ io.Writer) error {
			return extract(ctx, r, targetDir)
		},
	)

	if err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}

	return nil
}

func download(ctx context.Context, storageConfig config.Storage, filename string, target io.Writer) error {
	log.Println("Stage 1: Download")
	start := time.Now()
	defer func() {
		log.Printf("Stage 1 completed in %v", time.Since(start))
	}()

	u, err := storageConfig.GetURL()
	if err != nil {
		log.Printf("Download failed: failed to parse storage url: %v", err)
		return fmt.Errorf("failed to parse storage url: %w", err)
	}

	storageBackend, err := storage.New(u)
	if err != nil {
		log.Printf("Download failed: failed to create storage backend: %v", err)
		return fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Download the backup
	if downErr := storageBackend.Download(ctx, filename, target); downErr != nil {
		log.Printf("Download failed for %s: %v", filename, downErr)
		return fmt.Errorf("failed to download: %w", downErr)
	}

	return nil
}

func decrypt(ctx context.Context, config config.Encryption, source io.Reader, target io.Writer) error {
	if !config.Enabled() {
		_, err := io.Copy(target, source)
		if err != nil {
			return fmt.Errorf("failed to copy data: %w", err)
		}

		return nil
	}

	log.Println("Stage 1.1: Decrypt")
	start := time.Now()
	defer func() {
		log.Printf("Stage 1.1 completed in %v", time.Since(start))
	}()

	masterKey, err := config.Key()
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	service := encryption.NewAES256GCMService(masterKey)
	if decErr := service.Decrypt(ctx, source, target); decErr != nil {
		return fmt.Errorf("failed to decrypt: %w", decErr)
	}

	return nil
}

func extract(ctx context.Context, source io.Reader, targetdir string) error {
	log.Println("Stage 2: Decompress")
	start := time.Now()
	defer func() {
		log.Printf("Stage 2 completed in %v", time.Since(start))
	}()

	pigzCmd := exec.CommandContext(ctx, "pigz", "-d")
	tarCmd := exec.CommandContext(ctx, "tar", "-C", targetdir, "-xf", "-")

	// Set up piping
	var err error
	tarCmd.Stdin, err = pigzCmd.StdoutPipe()
	if err != nil {
		log.Printf("Decompression failed: failed to create stdout pipe: %v", err)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	pigzCmd.Stdin = source

	var tarErr, pigzErr bytes.Buffer
	var errs []error
	tarCmd.Stderr = &tarErr
	pigzCmd.Stderr = &pigzErr

	if pigzErr := pigzCmd.Start(); pigzErr != nil {
		log.Printf("Decompression failed: failed to start pigz: %v", pigzErr)
		return fmt.Errorf("failed to start pigz: %w", pigzErr)
	}
	if tarErr := tarCmd.Run(); tarErr != nil {
		log.Printf("Decompression failed: failed to run tar: %v", tarErr)
		errs = append(errs, fmt.Errorf("failed to run tar: %w", tarErr))
	}
	if waitErr := pigzCmd.Wait(); waitErr != nil {
		log.Printf("Decompression failed: failed to wait for pigz: %v", waitErr)
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
