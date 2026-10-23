package mongodb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/exec"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
)

const MethodName = "mongodump"

// Config carries the connection settings a mongodump backup needs.
type Config struct {
	Host          string
	Port          int
	User          string
	Password      string
	BackupOptions string
	BackupBinary  string
	MongoDB       config.MongoDB
}

// Method implements method.Method for mongodump (MongoDB logical backup).
type Method struct {
	config Config
}

// New creates a Method with the given MongoDB config.
func New(cfg Config) *Method {
	return &Method{config: cfg}
}

// MethodName returns the registry identifier for MongoDB backups.
func (m *Method) MethodName() string {
	return MethodName
}

// Backup runs a single mongodump invocation writing the dump into tempdir.
// Credentials are passed through a temporary config file created outside
// tempdir and removed after the run, so they never reach argv, logs, or the
// archived backup directory.
func (m *Method) Backup(ctx context.Context, tempdir string, logger logging.Logger) error {
	logger.Info(ctx, "Stage 1: Dump")
	start := time.Now()
	defer func() {
		logger.Info(ctx, "Stage 1 completed", logging.Fields{
			"duration": time.Since(start).String(),
		})
	}()

	opts, err := m.sanitizedOptions()
	if err != nil {
		return err
	}

	args, cleanup, err := m.buildArgs(tempdir, opts)
	if cleanup != nil {
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				logger.Error(ctx, "Failed to remove credentials file", cleanupErr, logging.Fields{})
			}
		}()
	}
	if err != nil {
		return err
	}

	if runErr := exec.Run(ctx, args, nil, os.Stdout); runErr != nil {
		return fmt.Errorf("mongodump failed: %w", runErr)
	}

	return nil
}

// Prepare is a no-op for mongodump backups.
func (m *Method) Prepare(_ context.Context, _ string, _ logging.Logger) error {
	return nil
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

// buildArgs assembles the mongodump command line. When credentials must stay
// off the command line it writes a 0600 config file in a temporary directory
// outside tempdir and returns a cleanup function that removes it again.
func (m *Method) buildArgs(tempdir string, opts []string) ([]string, func() error, error) {
	info := inspectURI(m.config.MongoDB.URI)
	needsFile := m.config.Password != "" || (info.set && info.hasCredentials)

	credsFile, cleanup, err := m.credentialsFile(needsFile, info)
	if err != nil {
		return nil, nil, err
	}

	args := []string{m.config.BackupBinary}
	args = append(args, m.connectionArgs(info, needsFile, credsFile)...)
	args = append(args, m.serverArgs(info)...)
	args = append(args, "--out="+tempdir)
	args = append(args, opts...)

	return args, cleanup, nil
}

// credentialsFile writes the temporary mongodump --config file when
// credentials must be kept off the command line. It returns an empty path and
// a nil cleanup function when no credentials file is needed.
func (m *Method) credentialsFile(needsFile bool, info uriInfo) (string, func() error, error) {
	if !needsFile {
		return "", nil, nil
	}

	path, err := writeCredentialsFile(m.config.MongoDB.URI, m.config.Password, info.set)
	if err != nil {
		return "", nil, err
	}

	cleanup := func() error {
		if rmErr := os.RemoveAll(filepath.Dir(path)); rmErr != nil {
			return fmt.Errorf("failed to remove credentials directory: %w", rmErr)
		}
		return nil
	}

	return path, cleanup, nil
}

// connectionArgs builds the connection part of argv: a URI when one is set,
// host/port/user otherwise, plus the --config flag when a credentials file
// exists.
func (m *Method) connectionArgs(info uriInfo, needsFile bool, credsFile string) []string {
	args := []string{}

	// A URI replaces host/port/user. When it may contain credentials it stays
	// in the config file so it never shows up in argv.
	if info.set && !needsFile {
		args = append(args, "--uri="+m.config.MongoDB.URI)
	}
	if !info.set {
		args = append(args, m.hostPortUserArgs()...)
	}
	if credsFile != "" {
		args = append(args, "--config="+credsFile)
	}

	return args
}

func (m *Method) hostPortUserArgs() []string {
	args := []string{}

	if m.config.Host != "" {
		args = append(args, "--host="+m.config.Host)
	}
	if m.config.Port != 0 {
		args = append(args, "--port="+strconv.Itoa(m.config.Port))
	}
	if m.config.User != "" {
		args = append(args, "--username="+m.config.User)
	}

	return args
}

// serverArgs builds the authentication and TLS flags.
func (m *Method) serverArgs(info uriInfo) []string {
	args := []string{}

	// Only pass --authenticationDatabase when there is a principal to
	// authenticate: without a username it makes mongodump attempt SCRAM
	// authentication and fail against servers that do not require auth.
	// Skip it when the URI already carries authSource; mongodump rejects
	// conflicting values.
	hasPrincipal := info.hasCredentials || (!info.set && m.config.User != "")
	if auth := m.config.MongoDB.AuthSource; auth != "" && hasPrincipal && (!info.set || !info.hasAuthSource) {
		args = append(args, "--authenticationDatabase="+auth)
	}
	if m.config.MongoDB.TLS {
		args = append(args, "--tls")
	}
	if caFile := m.config.MongoDB.TLSCAFile; caFile != "" {
		args = append(args, "--tlsCAFile="+caFile)
	}
	if certFile := m.config.MongoDB.TLSCertificateKeyFile; certFile != "" {
		args = append(args, "--tlsCertificateKeyFile="+certFile)
	}

	return args
}

// uriInfo describes the connection URI relevant to argument building.
type uriInfo struct {
	set            bool
	hasCredentials bool
	hasAuthSource  bool
}

// inspectURI reports whether a URI is set, whether it embeds a password, and
// whether it selects an authSource. Unparseable URIs are treated as secret so
// they are never placed on the command line.
func inspectURI(raw string) uriInfo {
	if raw == "" {
		return uriInfo{set: false, hasCredentials: false, hasAuthSource: false}
	}

	info := uriInfo{set: true, hasCredentials: false, hasAuthSource: false}

	u, err := url.Parse(raw)
	if err != nil {
		info.hasCredentials = true
		return info
	}

	if u.User != nil {
		_, info.hasCredentials = u.User.Password()
	}
	for key := range u.Query() {
		if strings.EqualFold(key, "authSource") {
			info.hasAuthSource = true
		}
	}

	return info
}

// writeCredentialsFile writes a mongodump --config file (YAML keys supported
// by the database tools: uri and password) with mode 0600 in a new temporary
// directory outside the backup tempdir.
func writeCredentialsFile(uri, password string, includeURI bool) (string, error) {
	dir, err := os.MkdirTemp("", "mongodump-creds")
	if err != nil {
		return "", fmt.Errorf("failed to create credentials directory: %w", err)
	}

	var content strings.Builder
	if includeURI && uri != "" {
		content.WriteString("uri: " + strconv.Quote(uri) + "\n")
	}
	if password != "" {
		content.WriteString("password: " + strconv.Quote(password) + "\n")
	}

	path := filepath.Join(dir, "credentials.yml")
	if writeErr := os.WriteFile(path, []byte(content.String()), 0o600); writeErr != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("failed to write credentials file: %w", writeErr)
	}

	return path, nil
}
