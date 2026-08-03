package logical

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/exec"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
)

const MethodName = "mariadb-dump"

// Method implements method.Method for mariadb-dump (logical/SQL backup).
type Method struct {
	config method.Config
}

// New creates a Method with the given MariaDB config.
func New(cfg method.Config) *Method {
	return &Method{config: cfg}
}

// MethodName returns the registry identifier for logical backups.
func (m *Method) MethodName() string {
	return MethodName
}

// Backup dumps each database as a separate .sql file into tempdir.
// Note: each database is dumped in its own mariadb-dump invocation, so the
// resulting .sql files are not point-in-time consistent across databases.
func (m *Method) Backup(ctx context.Context, tempdir string, logger logging.Logger) error {
	logger.Info(ctx, "Stage 1: Dump")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		logger.Info(ctx, "Stage 1 completed", logging.Fields{
			"duration": duration.String(),
		})
	}()

	databases, err := m.listDatabases(ctx)
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}

	logger.Info(ctx, "Databases found", logging.Fields{
		"count": strconv.Itoa(len(databases)),
	})

	for _, db := range databases {
		if dumpErr := m.dumpDatabase(ctx, tempdir, db, logger); dumpErr != nil {
			return fmt.Errorf("failed to dump database %q: %w", db, dumpErr)
		}
	}

	return nil
}

// Prepare is a no-op for logical backups.
func (m *Method) Prepare(_ context.Context, _ string, _ logging.Logger) error {
	return nil
}

// sharedOptionPrefixes lists BackupOptions flags that control the client
// connection and must be applied to both database discovery and dumps.
func isSharedConnectionOption(opt string) bool {
	for _, prefix := range []string{"--socket", "--ssl", "--tls-", "--default-auth"} {
		if strings.HasPrefix(opt, prefix) {
			return true
		}
	}
	return false
}

func (m *Method) sanitizedOptions() ([]string, error) {
	if m.config.BackupOptions == "" {
		return nil, nil
	}

	opts, err := sanitizer.SanitizeOptions(m.config.BackupOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to sanitize options: %w", err)
	}

	return opts, nil
}

func (m *Method) connectionArgs() ([]string, error) {
	args := []string{}

	if m.config.Host != "" {
		args = append(args, "--host="+m.config.Host)
	}

	if m.config.Port != 0 {
		args = append(args, "--port="+strconv.Itoa(m.config.Port))
	}

	args = append(args, "--user="+m.config.User)

	opts, err := m.sanitizedOptions()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if isSharedConnectionOption(opt) {
			args = append(args, opt)
		}
	}

	return args, nil
}

func (m *Method) dumpOnlyArgs() ([]string, error) {
	opts, err := m.sanitizedOptions()
	if err != nil {
		return nil, err
	}

	args := make([]string, 0, len(opts))
	for _, opt := range opts {
		if !isSharedConnectionOption(opt) {
			args = append(args, opt)
		}
	}

	return args, nil
}

func (m *Method) listDatabases(ctx context.Context) ([]string, error) {
	connArgs, err := m.connectionArgs()
	if err != nil {
		return nil, err
	}

	args := []string{
		m.config.ClientBinary,
		"-N",
		"-e", "SHOW DATABASES",
	}
	args = append(args, connArgs...)

	var stdout bytes.Buffer
	if runErr := exec.Run(ctx, args, map[string]string{"MYSQL_PWD": m.config.Password}, &stdout); runErr != nil {
		return nil, fmt.Errorf("failed to list databases: %w", runErr)
	}

	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		return nil, nil
	}

	databases := make([]string, 0)
	for db := range strings.SplitSeq(raw, "\n") {
		switch db {
		case "information_schema", "performance_schema":
			continue
		default:
			databases = append(databases, db)
		}
	}

	return databases, nil
}

func (m *Method) dumpDatabase(ctx context.Context, tempdir string, db string, logger logging.Logger) error {
	logger.Info(ctx, "Dumping database", logging.Fields{"database": db})

	connArgs, err := m.connectionArgs()
	if err != nil {
		return err
	}
	dumpArgs, err := m.dumpOnlyArgs()
	if err != nil {
		return err
	}

	args := []string{
		m.config.BackupBinary,
		"--databases", db,
		"--routines",
		"--triggers",
		"--events",
	}
	args = append(args, connArgs...)
	args = append(args, dumpArgs...)

	outFile, err := os.Create(filepath.Join(tempdir, db+".sql"))
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		if closeErr := outFile.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to close output file", closeErr, logging.Fields{
				"database": db,
			})
		}
	}()

	if execErr := exec.Run(ctx, args, map[string]string{"MYSQL_PWD": m.config.Password}, outFile); execErr != nil {
		return fmt.Errorf("mariadb-dump failed: %w", execErr)
	}

	return nil
}
