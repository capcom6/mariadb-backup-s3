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

	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

var cores = runtime.NumCPU()

func Execute(ctx context.Context, cfg Config) error {
	tempdir, err := os.MkdirTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create tempdir: %w", err)
	}
	defer os.RemoveAll(tempdir)
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
	defer os.Remove(compressed.Name())

	if err := compress(ctx, tempdir, compressed.Name()); err != nil {
		return fmt.Errorf("failed to compress: %w", err)
	}
	log.Printf("compressed done: %s", compressed.Name())

	if err := upload(ctx, cfg.Backup, cfg.Storage, compressed.Name()); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	log.Printf("upload done: %s", compressed.Name())

	return nil
}

func run(_ context.Context, cmdline string) error {
	buf := bytes.Buffer{}

	cmd := exec.Command("bash", "-c", cmdline)

	cmd.Stdout = os.Stdout
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return errors.Join(errors.New(buf.String()), err)
	}
	return nil
}

func backup(ctx context.Context, options MariaDBConfig, dir string) error {
	cmdline := fmt.Sprintf(`mariabackup --backup --parallel=%d --target-dir='%s' --user='%s' --password='%s'`, cores, dir, options.User, options.Password)
	if options.Host != "" {
		cmdline += fmt.Sprintf(" --host='%s' --port=%d", options.Host, options.Port)
	}
	if options.BackupOptions != "" {
		cmdline += fmt.Sprintf(" %s", options.BackupOptions)
	}

	return run(ctx, cmdline)
}

func prepare(ctx context.Context, _ MariaDBConfig, dir string) error {
	cmdline := fmt.Sprintf(`mariabackup --prepare --target-dir='%s'`, dir)

	return run(ctx, cmdline)
}

func compress(ctx context.Context, source, target string) error {
	cmdline := fmt.Sprintf(`tar -cf - '%s' | pigz > '%s'`, source, target)

	return run(ctx, cmdline)
}

func upload(ctx context.Context, backup Backup, storageConfig StorageConfig, source string) error {
	filename := time.Now().UTC().Format("2006-01-02-15-04-05") + ".tar.gz"

	u, err := storageConfig.GetURL()
	if err != nil {
		return fmt.Errorf("failed to parse storage url: %w", err)
	}

	storageBackend, err := storage.New(u)
	if err != nil {
		return fmt.Errorf("failed to create storage backend: %w", err)
	}

	h, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", source, err)
	}
	defer h.Close()

	// Upload the backup
	if err := storageBackend.Upload(ctx, filename, h); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	// Cleanup old backups
	if err := storageBackend.DeleteOldBackups(ctx, backup.Limits.MaxCount); err != nil {
		log.Printf("failed to cleanup: %s", err)
	}

	return nil
}

func isInterrupted(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
