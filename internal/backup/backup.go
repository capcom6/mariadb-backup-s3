package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

var cores = runtime.NumCPU()

func Execute(ctx context.Context, cfg Config) error {
	tempdir, err := os.MkdirTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create tempdir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempdir); err != nil {
			log.Printf("failed to remove tempdir: %s", err)
		}
	}()
	log.Printf("tempdir: %s", tempdir)
	log.Printf("Starting backup pipeline")

	if err := backup(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to backup: %w", err)
	}
	log.Printf("backup done: %s", tempdir)

	if err := prepare(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to prepare: %w", err)
	}
	log.Printf("prepare done: %s", tempdir)

	// Create pipe for streaming compression to upload
	pipeReader, pipeWriter := io.Pipe()
	defer func(pipeReader *io.PipeReader) {
		if err := pipeReader.Close(); err != nil {
			log.Printf("failed to close pipe: %s", err)
		}
	}(pipeReader)

	backupPath := time.Now().UTC().Format("2006-01-02-15-04-05") + ".tar.gz"

	// Start compression in goroutine
	go func() {
		err := compress(ctx, tempdir, pipeWriter)
		if err != nil {
			if closeErr := pipeWriter.CloseWithError(err); closeErr != nil {
				log.Printf("failed to close pipe with error: %s", closeErr)
			}
			return
		}
		if err := pipeWriter.Close(); err != nil {
			log.Printf("failed to close pipe: %s", err)
		}
	}()

	log.Printf("Starting upload to %s", backupPath)
	if err := upload(ctx, cfg.Backup, cfg.Storage, pipeReader, backupPath); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	log.Printf("Backup pipeline completed successfully")
	return nil
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}

	buf := bytes.Buffer{}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return errors.Join(errors.New(buf.String()), err)
	}
	return nil
}

func backup(ctx context.Context, options MariaDBConfig, dir string) error {
	args := []string{
		"mariabackup",
		"--backup",
		"--parallel=" + fmt.Sprintf("%d", cores),
		"--target-dir=" + dir,
		"--user=" + options.User,
		"--password=" + options.Password,
	}

	if options.Host != "" {
		args = append(args, "--host="+options.Host, "--port="+fmt.Sprintf("%d", options.Port))
	}

	if options.BackupOptions != "" {
		opts, err := sanitizer.SanitizeOptions(options.BackupOptions)
		if err != nil {
			return fmt.Errorf("failed to sanitize options: %w", err)
		}

		args = append(args, opts...)
	}

	return run(ctx, args)
}

func prepare(ctx context.Context, _ MariaDBConfig, dir string) error {
	args := []string{
		"mariabackup",
		"--prepare",
		"--target-dir=" + dir,
	}

	return run(ctx, args)
}

func compress(ctx context.Context, source string, target io.Writer) error {
	start := time.Now()
	log.Printf("Starting compression of %s", source)

	// Create tar command
	tarCmd := exec.CommandContext(ctx, "tar", "-C", source, "-cf", "-", ".")
	pigzCmd := exec.CommandContext(ctx, "pigz")

	// Set up piping
	var err error
	pigzCmd.Stdin, err = tarCmd.StdoutPipe()
	if err != nil {
		log.Printf("Compression failed: failed to create stdout pipe: %v", err)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	pigzCmd.Stdout = target

	var tarErr, pigzErr bytes.Buffer
	var errs []error
	tarCmd.Stderr = &tarErr
	pigzCmd.Stderr = &pigzErr

	if err := pigzCmd.Start(); err != nil {
		log.Printf("Compression failed: failed to start pigz: %v", err)
		return fmt.Errorf("failed to start pigz: %w", err)
	}
	if err := tarCmd.Run(); err != nil {
		errs = append(errs, errors.Join(errors.New(tarErr.String()), err))
	}
	if err := pigzCmd.Wait(); err != nil {
		errs = append(errs, errors.Join(errors.New(pigzErr.String()), err))
	}

	duration := time.Since(start)
	if len(errs) > 0 {
		log.Printf("Compression failed after %v: %v", duration, errors.Join(errs...))
	} else {
		log.Printf("Compression completed successfully in %v", duration)
	}
	return errors.Join(errs...)
}

func upload(ctx context.Context, backup Backup, storageConfig StorageConfig, source io.Reader, filename string) error {
	start := time.Now()
	u, err := storageConfig.GetURL()
	if err != nil {
		log.Printf("Upload failed: failed to parse storage url: %v", err)
		return fmt.Errorf("failed to parse storage url: %w", err)
	}

	storageBackend, err := storage.New(u)
	if err != nil {
		log.Printf("Upload failed: failed to create storage backend: %v", err)
		return fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Upload the backup
	if err := storageBackend.Upload(ctx, filename, source); err != nil {
		log.Printf("Upload failed for %s: %v", filename, err)
		return fmt.Errorf("failed to upload: %w", err)
	}

	duration := time.Since(start)

	// Cleanup old backups
	if err := storageBackend.DeleteOldBackups(ctx, backup.Limits.MaxCount); err != nil {
		log.Printf("failed to cleanup: %s", err)
	}

	log.Printf("Upload completed successfully: %s in %v", filename, duration)

	return nil
}
