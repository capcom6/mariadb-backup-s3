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

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
)

var cores = runtime.NumCPU()

func Execute(ctx context.Context, cfg Config) error {
	log.Println("Starting backup")
	start := time.Now()
	defer func() {
		log.Printf("Backup completed in %v", time.Since(start))
	}()

	targetName := start.UTC().Format("2006-01-02-15-04-05") + ".tar.gz"
	if cfg.Encryption.Enabled() {
		targetName += ".enc"
	}

	tempdir, err := os.MkdirTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create tempdir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempdir); err != nil {
			log.Printf("failed to remove tempdir: %s", err)
		}
	}()

	if err := backup(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to backup: %w", err)
	}
	log.Printf("backup done: %s", tempdir)

	if err := prepare(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to prepare: %w", err)
	}
	log.Printf("prepare done: %s", tempdir)

	return pipeline.Run(ctx, nil, nil,
		func(ctx context.Context, _ io.Reader, w io.Writer) error {
			return compress(ctx, tempdir, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return encrypt(ctx, cfg.Encryption, r, w)
		},
		func(ctx context.Context, r io.Reader, _ io.Writer) error {
			return upload(ctx, cfg.Backup, cfg.Storage, r, targetName)
		},
	)
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
	log.Println("Stage 1: Backup")
	start := time.Now()
	defer func() {
		log.Printf("Stage 1 completed in %v", time.Since(start))
	}()

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
	log.Println("Stage 2: Prepare")
	start := time.Now()
	defer func() {
		log.Printf("Stage 2 completed in %v", time.Since(start))
	}()

	args := []string{
		"mariabackup",
		"--prepare",
		"--target-dir=" + dir,
	}

	return run(ctx, args)
}

func compress(ctx context.Context, source string, target io.Writer) error {
	log.Println("Stage 3: Compress")
	start := time.Now()
	defer func() {
		log.Printf("Stage 3 completed in %v", time.Since(start))
	}()

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

	return errors.Join(errs...)
}

func encrypt(ctx context.Context, config EncryptionConfig, source io.Reader, target io.Writer) error {
	if !config.Enabled() {
		_, err := io.Copy(target, source)
		if err != nil {
			return fmt.Errorf("failed to copy data: %w", err)
		}

		return nil
	}

	log.Println("Stage 3.1: Encrypt")
	start := time.Now()
	defer func() {
		log.Printf("Stage 3.1 completed in %v", time.Since(start))
	}()

	masterKey, err := config.Key()
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	service := encryption.NewAES256GCMService(masterKey)

	return service.Encrypt(ctx, source, target)
}

func upload(ctx context.Context, backup Backup, storageConfig StorageConfig, source io.Reader, filename string) error {
	log.Println("Stage 4: Upload")
	start := time.Now()
	defer func() {
		log.Printf("Stage 4 completed in %v", time.Since(start))
	}()

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

	// Cleanup old backups
	if err := storageBackend.DeleteOldBackups(ctx, backup.Limits.MaxCount); err != nil {
		log.Printf("failed to cleanup: %s", err)
	}

	return nil
}
