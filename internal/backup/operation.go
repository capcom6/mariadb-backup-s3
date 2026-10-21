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

	// streamer is set when the backup is produced as a stream, instead of
	// files in a temp directory.
	streamer method.StreamingMethod

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
	var s method.StreamingMethod

	mConfig := method.Config{
		Host:          config.MariaDB.Host,
		Port:          config.MariaDB.Port,
		User:          config.MariaDB.User,
		Password:      config.MariaDB.Password,
		BackupOptions: config.MariaDB.BackupOptions,
		BackupBinary:  config.MariaDB.BackupBinary,
		ClientBinary:  config.MariaDB.ClientBinary,
		Stream:        config.Stream,
	}
	if config.MariaDB.IsLogical() {
		m = logical.New(mConfig)
	} else {
		m = physical.New(mConfig)
	}

	if str, ok := m.(method.StreamingMethod); config.Stream && ok {
		s = str
	}

	return &Operation{
		config: config,

		method: m,

		streamer: s,

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

	if o.streamer != nil {
		return o.runStreaming(ctx, start)
	}

	return o.runToFiles(ctx, start)
}

// runToFiles runs the backup through a temp directory: the method writes its
// output files there, and the directory is compressed, encrypted and uploaded.
func (o *Operation) runToFiles(ctx context.Context, start time.Time) error {
	targetName := o.targetName(start, o.method.ArtifactExtension())

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

// runStreaming runs the backup without touching the local disk: the method
// writes its output into the pipeline, which compresses, encrypts and uploads
// it.
func (o *Operation) runStreaming(ctx context.Context, start time.Time) error {
	o.logger.Info(ctx, "Streaming backup to storage")

	targetName := o.targetName(start, o.streamer.ArtifactExtension())

	err := pipeline.Run(ctx, nil, nil,
		func(ctx context.Context, _ io.Reader, w io.Writer) error {
			return o.dump(ctx, w)
		},
		func(ctx context.Context, r io.Reader, w io.Writer) error {
			return o.compressStream(ctx, r, w)
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

// targetName builds the name of the artifact for a backup.
func (o *Operation) targetName(start time.Time, extension string) string {
	name := start.UTC().Format("2006-01-02-15-04-05") + "." + extension
	if o.config.Encryption.Enabled() {
		name += ".enc"
	}

	return name
}

// dump writes the backup content into the pipeline.
func (o *Operation) dump(ctx context.Context, w io.Writer) error {
	if dumpErr := o.streamer.Stream(ctx, w, o.logger); dumpErr != nil {
		return fmt.Errorf("failed to dump: %w", dumpErr)
	}

	return nil
}

// compressStream compresses the stream with pigz.
func (o *Operation) compressStream(ctx context.Context, r io.Reader, w io.Writer) error {
	o.logger.Info(ctx, "Stage 2: Compress")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 2 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	pigzCmd := osexec.CommandContext(ctx, "pigz")
	pigzCmd.Stdin = r
	pigzCmd.Stdout = w

	var stderr bytes.Buffer
	pigzCmd.Stderr = &stderr

	if compressErr := pigzCmd.Run(); compressErr != nil {
		return fmt.Errorf("failed to compress: %s: %w", stderr.String(), compressErr)
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
