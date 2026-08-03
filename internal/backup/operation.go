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
	osexec "os/exec"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/logical"
	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/physical"
	"github.com/capcom6/mariadb-backup-s3/internal/encryption"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/pkg/counting"
	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
)

const (
	logFieldDuration = "duration"
	logFieldFilename = "filename"
	logFieldSource   = "source"
	logFieldTempdir  = "tempdir"
)

type Operation struct {
	config Config

	method method.Method

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
	var m method.Method
	mConfig := method.Config{
		Host:          config.MariaDB.Host,
		Port:          config.MariaDB.Port,
		User:          config.MariaDB.User,
		Password:      config.MariaDB.Password,
		BackupOptions: config.MariaDB.BackupOptions,
		BackupBinary:  config.MariaDB.BackupBinary,
		ClientBinary:  config.MariaDB.ClientBinary,
	}
	if config.MariaDB.IsLogical() {
		m = logical.New(mConfig)
	} else {
		m = physical.New(mConfig)
	}

	return &Operation{
		config: config,

		method: m,

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
			logFieldDuration: duration.String(),
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
				logFieldTempdir: tempdir,
			})
		}
	}()

	if bkpErr := o.method.Backup(ctx, tempdir, o.logger); bkpErr != nil {
		return fmt.Errorf("failed to backup: %w", bkpErr)
	}
	o.logger.Info(ctx, "backup done", logging.Fields{
		logFieldTempdir: tempdir,
	})

	if prepErr := o.method.Prepare(ctx, tempdir, o.logger); prepErr != nil {
		return fmt.Errorf("failed to prepare: %w", prepErr)
	}
	o.logger.Info(ctx, "prepare done", logging.Fields{
		logFieldTempdir: tempdir,
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

func (o *Operation) compress(ctx context.Context, tempdir string, w io.Writer) error {
	o.logger.Info(ctx, "Stage 3: Compress")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 3 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	// Create tar command
	tarCmd := osexec.CommandContext(ctx, "tar", "-C", tempdir, "-cf", "-", ".")
	pigzCmd := osexec.CommandContext(ctx, "pigz")

	// Set up piping
	var err error
	pigzCmd.Stdin, err = tarCmd.StdoutPipe()
	if err != nil {
		o.logger.Error(ctx, "Compression failed: failed to create stdout pipe", err, logging.Fields{
			logFieldSource: tempdir,
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
			logFieldSource: tempdir,
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
			logFieldDuration: duration.String(),
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
			logFieldDuration: duration.String(),
		})
	}()

	shaSum := sha256.New()
	counter := counting.NewWriter()
	uploadReader := io.TeeReader(r, io.MultiWriter(shaSum, counter))

	// Upload the backup
	if uploadErr := o.storage.Upload(ctx, targetName, uploadReader); uploadErr != nil {
		o.logger.Error(ctx, "Upload failed", uploadErr, logging.Fields{
			logFieldFilename: targetName,
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
			Method:  o.method.MethodName(),
		},
	)

	if o.config.Encryption.Enabled() {
		entry.Encryption = &registry.EncryptionMetadata{Algorithm: "AES-256-GCM"}
	}

	if _, err := o.registrySvc.Append(ctx, entry); err != nil {
		o.logger.Error(ctx, "registry append failed", err, logging.Fields{
			logFieldFilename: targetName,
		})
		return fmt.Errorf("failed to append registry: %w", err)
	}

	return nil
}
