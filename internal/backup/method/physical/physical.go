package physical

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/exec"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
)

const MethodName = "mariadb-backup"

// Method implements method.Method for mariadb-backup (physical/hot backup).
type Method struct {
	config method.Config
}

// New creates a Method with the given MariaDB config.
func New(cfg method.Config) *Method {
	return &Method{config: cfg}
}

// MethodName returns the registry identifier for physical backups.
func (m *Method) MethodName() string {
	return MethodName
}

// Backup executes mariadb-backup --backup, writing data files into tempdir.
func (m *Method) Backup(ctx context.Context, tempdir string, logger logging.Logger) error {
	logger.Info(ctx, "Stage 1: Backup")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		logger.Info(ctx, "Stage 1 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	args := []string{
		m.config.BackupBinary,
		"--backup",
		"--parallel=" + strconv.Itoa(runtime.NumCPU()),
		"--target-dir=" + tempdir,
		"--user=" + m.config.User,
	}

	if m.config.Host != "" {
		args = append(args, "--host="+m.config.Host)
	}

	if m.config.Port != 0 {
		args = append(args, "--port="+strconv.Itoa(m.config.Port))
	}

	if m.config.BackupOptions != "" {
		opts, err := sanitizer.SanitizeOptions(m.config.BackupOptions)
		if err != nil {
			return fmt.Errorf("failed to sanitize options: %w", err)
		}

		args = append(args, opts...)
	}

	if err := exec.Run(ctx, args, map[string]string{"MYSQL_PWD": m.config.Password}, os.Stdout); err != nil {
		return fmt.Errorf("mariadb-backup --backup failed: %w", err)
	}

	return nil
}

// Prepare executes mariadb-backup --prepare on the backup data in tempdir.
func (m *Method) Prepare(ctx context.Context, tempdir string, logger logging.Logger) error {
	logger.Info(ctx, "Stage 2: Prepare")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		logger.Info(ctx, "Stage 2 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	args := []string{
		m.config.BackupBinary,
		"--prepare",
		"--target-dir=" + tempdir,
	}

	if err := exec.Run(ctx, args, nil, os.Stdout); err != nil {
		return fmt.Errorf("mariadb-backup --prepare failed: %w", err)
	}

	return nil
}
