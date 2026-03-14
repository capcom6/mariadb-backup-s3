package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/counting"
	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
)

type Operation struct {
	config Config

	registrySvc *registry.Service

	storage storage.Backend

	logger logging.Logger
}

func NewOperation(
	config Config,
	registrySvc *registry.Service,
	storage storage.Backend,
	logger logging.Logger,
) *Operation {
	return &Operation{
		config: config,

		registrySvc: registrySvc,

		storage: storage,

		logger: logger,
	}
}

func (o *Operation) Run(ctx context.Context) error {
	ctx = logging.WithComponent(ctx, "backup")

	o.logger.Info(ctx, "Starting backup")

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Backup completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	targetName := start.UTC().Format("2006-01-02-15-04-05") + ".tar.gz"
	if o.config.Encryption.Enabled() {
		targetName += ".enc"
	}

	tempdir, err := os.MkdirTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create tempdir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tempdir); rmErr != nil {
			o.logger.Error(ctx, "failed to remove tempdir", rmErr, logging.Fields{
				"tempdir": tempdir,
			})
		}
	}()

	if bkpErr := o.backup(ctx, tempdir); bkpErr != nil {
		return fmt.Errorf("failed to backup: %w", bkpErr)
	}
	o.logger.Info(ctx, "backup done", logging.Fields{
		"tempdir": tempdir,
	})

	if prepErr := o.prepare(ctx, tempdir); prepErr != nil {
		return fmt.Errorf("failed to prepare: %w", prepErr)
	}
	o.logger.Info(ctx, "prepare done", logging.Fields{
		"tempdir": tempdir,
	})

	err = pipeline.Run(ctx, nil, nil,
		func(ctx context.Context, _ io.Reader, w io.Writer) error {
			return o.compress(ctx, tempdir, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return o.encrypt(ctx, r, w)
		},
		func(ctx context.Context, r io.Reader, _ io.Writer) error {
			return o.upload(ctx, r, targetName, start.UTC())
		},
	)

	if err != nil {
		return fmt.Errorf("failed to backup: %w", err)
	}

	return nil
}

func (o *Operation) backup(ctx context.Context, tempdir string) error {
	o.logger.Info(ctx, "Stage 1: Backup")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 1 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	args := []string{
		o.config.MariaDB.BackupBinary,
		"--backup",
		"--parallel=" + strconv.Itoa(runtime.NumCPU()),
		"--target-dir=" + tempdir,
		"--user=" + o.config.MariaDB.User,
		"--password=" + o.config.MariaDB.Password,
	}

	if o.config.MariaDB.Host != "" {
		args = append(args, "--host="+o.config.MariaDB.Host)
	}

	if o.config.MariaDB.Port != 0 {
		args = append(args, "--port="+strconv.Itoa(o.config.MariaDB.Port))
	}

	if o.config.MariaDB.BackupOptions != "" {
		opts, err := sanitizer.SanitizeOptions(o.config.MariaDB.BackupOptions)
		if err != nil {
			return fmt.Errorf("failed to sanitize options: %w", err)
		}

		args = append(args, opts...)
	}

	return run(ctx, args)
}

func (o *Operation) prepare(ctx context.Context, tempdir string) error {
	o.logger.Info(ctx, "Stage 2: Prepare")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 2 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	args := []string{
		o.config.MariaDB.BackupBinary,
		"--prepare",
		"--target-dir=" + tempdir,
	}

	return run(ctx, args)
}

func (o *Operation) compress(ctx context.Context, tempdir string, w io.Writer) error {
	o.logger.Info(ctx, "Stage 3: Compress")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 3 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	// Create tar command
	tarCmd := exec.CommandContext(ctx, "tar", "-C", tempdir, "-cf", "-", ".")
	pigzCmd := exec.CommandContext(ctx, "pigz")

	// Set up piping
	var err error
	pigzCmd.Stdin, err = tarCmd.StdoutPipe()
	if err != nil {
		o.logger.Error(ctx, "Compression failed: failed to create stdout pipe", err, logging.Fields{
			"source": tempdir,
		})
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	pigzCmd.Stdout = w

	var tarBuf, pigzBuf bytes.Buffer
	var errs []error
	tarCmd.Stderr = &tarBuf
	pigzCmd.Stderr = &pigzBuf

	if pigzErr := pigzCmd.Start(); pigzErr != nil {
		o.logger.Error(ctx, "Compression failed: failed to start pigz", pigzErr, logging.Fields{
			"source": tempdir,
		})
		return fmt.Errorf("failed to start pigz: %w", pigzErr)
	}
	if tarErr := tarCmd.Run(); tarErr != nil {
		errs = append(errs, fmt.Errorf("failed to tar: %s: %w", tarBuf.String(), tarErr))
	}
	if waitErr := pigzCmd.Wait(); waitErr != nil {
		errs = append(errs, fmt.Errorf("failed to compress: %s: %w", pigzBuf.String(), waitErr))
	}

	return errors.Join(errs...)
}

func (o *Operation) encrypt(ctx context.Context, r io.Reader, w io.Writer) error {
	if !o.config.Encryption.Enabled() {
		_, err := io.Copy(w, r)
		if err != nil {
			return fmt.Errorf("failed to copy data: %w", err)
		}

		return nil
	}

	o.logger.Info(ctx, "Stage 3.1: Encrypt")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 3.1 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	masterKey, err := o.config.Encryption.Key()
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	service := encryption.NewAES256GCMService(masterKey)

	if encErr := service.Encrypt(ctx, r, w); encErr != nil {
		return fmt.Errorf("failed to encrypt: %w", encErr)
	}

	return nil
}

func (o *Operation) upload(ctx context.Context, r io.Reader, targetName string, createdAt time.Time) error {
	o.logger.Info(ctx, "Stage 4: Upload")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 4 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	shaSum := sha256.New()
	counter := counting.NewWriter()
	uploadReader := io.TeeReader(r, io.MultiWriter(shaSum, counter))

	// Upload the backup
	if uploadErr := o.storage.Upload(ctx, targetName, uploadReader); uploadErr != nil {
		o.logger.Error(ctx, "Upload failed", uploadErr, logging.Fields{
			"filename": targetName,
		})
		return fmt.Errorf("failed to upload: %w", uploadErr)
	}

	entry := registry.NewBackupEntry(
		targetName,
		createdAt,
		counter.N(),
		hex.EncodeToString(shaSum.Sum(nil)),
		registry.StatusReady,
		o.config.Encryption.Enabled(),
		nil,
		&registry.ToolMetadata{
			Name:    "mariadb-backup-s3",
			Version: o.config.Version,
		},
	)

	if o.config.Encryption.Enabled() {
		entry.Encryption = &registry.EncryptionMetadata{Algorithm: "AES-256-GCM"}
	}

	if _, err := o.registrySvc.Append(ctx, entry); err != nil {
		o.logger.Error(ctx, "registry append failed", err, logging.Fields{
			"filename": targetName,
		})
		return fmt.Errorf("failed to append registry: %w", err)
	}

	return nil
}
