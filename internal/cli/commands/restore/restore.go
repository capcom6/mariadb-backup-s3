package restore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	fl := flags.Storage()
	fl = append(fl, flags.Encryption()...)
	fl = append(fl,
		&cli.StringFlag{
			Name:     "target-dir",
			Usage:    "target directory",
			Required: true,
			Sources:  cli.EnvVars("RESTORE__TARGET_DIR"),
		},
	)

	return &cli.Command{
		Name:  "restore",
		Usage: "Restore MariaDB database from backup",
		Flags: fl,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "backup-file",
				Value:     "",
				UsageText: "backup file name",
				Config: cli.StringConfig{
					TrimSpace: true,
				},
			},
		},
		ArgsUsage: "backup_name.tar.gz",
		Action: func(c context.Context, cmd *cli.Command) error {
			cfg := DefaultConfig()

			cfg.MariaDB.Host = cmd.String("db-host")
			cfg.MariaDB.Port = cmd.Int("db-port")
			cfg.MariaDB.User = cmd.String("db-user")
			cfg.MariaDB.Password = cmd.String("db-password")

			cfg.Storage.URL = cmd.String("storage-url")

			cfg.Encryption.EncryptionKey = cmd.String("encryption-key")

			cfg.Restore.TargetDir = cmd.String("target-dir")

			return Execute(c, cmd.String("backup-file"), cfg)
		},
	}
}

func Execute(ctx context.Context, filename string, cfg Config) error {
	log.Println("Starting restore")
	start := time.Now()
	defer func() {
		log.Printf("Restore completed in %v", time.Since(start))
	}()

	targetDir := cfg.Restore.TargetDir
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	return pipeline.Run(
		ctx,
		nil,
		nil,
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return download(ctx, cfg.Storage, filename, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return decrypt(ctx, cfg.Encryption, r, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return extract(ctx, r, targetDir)
		},
	)

	// tempdir, err := os.MkdirTemp("", "mariadb-restore")
	// if err != nil {
	// 	return fmt.Errorf("failed to create tempdir: %w", err)
	// }
	// defer func() {
	// 	if err := os.RemoveAll(tempdir); err != nil {
	// 		log.Printf("failed to remove tempdir: %s", err)
	// 	}
	// }()

	// // Create a temporary file for the downloaded backup
	// tempFile, err := os.CreateTemp("", "backup-download")
	// if err != nil {
	// 	return fmt.Errorf("failed to create temp file: %w", err)
	// }
	// defer os.Remove(tempFile.Name())
	// defer tempFile.Close()

	// // Download the backup
	// if err := download(ctx, cfg.Storage, cfg.Backup, tempFile); err != nil {
	// 	return err
	// }

	// // Reset file pointer to beginning for reading
	// if _, err := tempFile.Seek(0, 0); err != nil {
	// 	return fmt.Errorf("failed to seek temp file: %w", err)
	// }

	// // Decrypt and decompress to temp directory
	// if err := decrypt(ctx, cfg.Encryption, tempFile, tempdir); err != nil {
	// 	return err
	// }

	// // Restore from temp directory
	// return restore(ctx, cfg.MariaDB, cfg.Restore, tempdir)

	// return nil
}

func download(ctx context.Context, storageConfig StorageConfig, filename string, target io.Writer) error {
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
	if err := storageBackend.Download(ctx, filename, target); err != nil {
		log.Printf("Download failed for %s: %v", filename, err)
		return fmt.Errorf("failed to download: %w", err)
	}

	return nil
}

func decrypt(ctx context.Context, config EncryptionConfig, source io.Reader, target io.Writer) error {
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
	if err := service.Decrypt(ctx, source, target); err != nil {
		return fmt.Errorf("failed to decrypt: %w", err)
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

	if err := pigzCmd.Start(); err != nil {
		log.Printf("Decompression failed: failed to start pigz: %v", err)
		return fmt.Errorf("failed to start pigz: %w", err)
	}
	if err := tarCmd.Run(); err != nil {
		log.Printf("Decompression failed: failed to run tar: %v", err)
		errs = append(errs, fmt.Errorf("failed to run tar: %w", err))
	}
	if err := pigzCmd.Wait(); err != nil {
		log.Printf("Decompression failed: failed to wait for pigz: %v", err)
		errs = append(errs, fmt.Errorf("failed to wait for pigz: %w", err))
	}

	if len(tarErr.Bytes()) > 0 {
		errs = append(errs, fmt.Errorf("failed to extract tar: %s", tarErr.String()))
	}
	if len(pigzErr.Bytes()) > 0 {
		errs = append(errs, fmt.Errorf("failed to decompress: %s", pigzErr.String()))
	}

	return errors.Join(errs...)
}
