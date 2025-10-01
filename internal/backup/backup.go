package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

	if err := backup(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to backup: %w", err)
	}
	log.Printf("backup done: %s", tempdir)

	if err := prepare(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to prepare: %w", err)
	}
	log.Printf("prepare done: %s", tempdir)

	compressed, err := os.CreateTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create compressed: %w", err)
	}
	defer func(compressed *os.File) {
		if err := compressed.Close(); err != nil {
			log.Printf("failed to close compressed: %s", err)
		}
		if err := os.Remove(compressed.Name()); err != nil {
			log.Printf("failed to remove compressed: %s", err)
		}
	}(compressed)

	if err := compress(ctx, tempdir, compressed); err != nil {
		return fmt.Errorf("failed to compress: %w", err)
	}
	log.Printf("compressed done: %s", compressed.Name())

	if err := upload(ctx, cfg.Backup, cfg.Storage, compressed); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	log.Printf("upload done: %s", compressed.Name())

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

func compress(ctx context.Context, source string, target *os.File) error {
	// Create tar command
	tarCmd := exec.CommandContext(ctx, "tar", "-cf", "-", source)
	pigzCmd := exec.CommandContext(ctx, "pigz")

	// Set up piping
	var err error
	pigzCmd.Stdin, err = tarCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	pigzCmd.Stdout = target

	var tarErr, pigzErr bytes.Buffer
	var errs []error
	tarCmd.Stderr = &tarErr
	pigzCmd.Stderr = &pigzErr

	if err := pigzCmd.Start(); err != nil {
		return fmt.Errorf("failed to start pigz: %w", err)
	}
	if err := tarCmd.Run(); err != nil {
		errs = append(errs, errors.Join(errors.New(tarErr.String()), err))
	}
	if err := pigzCmd.Wait(); err != nil {
		errs = append(errs, errors.Join(errors.New(pigzErr.String()), err))
	}
	return errors.Join(errs...)
}

func upload(ctx context.Context, backup Backup, storageConfig StorageConfig, source *os.File) error {
	filename := time.Now().UTC().Format("2006-01-02-15-04-05") + ".tar.gz"

	u, err := storageConfig.GetURL()
	if err != nil {
		return fmt.Errorf("failed to parse storage url: %w", err)
	}

	storageBackend, err := storage.New(u)
	if err != nil {
		return fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Upload the backup
	if _, err := source.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	if err := storageBackend.Upload(ctx, filename, source); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	// Cleanup old backups
	if err := storageBackend.DeleteOldBackups(ctx, backup.Limits.MaxCount); err != nil {
		log.Printf("failed to cleanup: %s", err)
	}

	return nil
}
