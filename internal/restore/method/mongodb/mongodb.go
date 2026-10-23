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

// Config carries the connection settings a mongorestore restore needs.
type Config struct {
	Host           string
	Port           int
	User           string
	Password       string
	RestoreOptions string
	RestoreBinary  string
	MongoDB        config.MongoDB
}

// Method implements method.Method for mongorestore (MongoDB logical restore).
type Method struct {
	config Config
}

// New creates a Method with the given MongoDB config.
func New(cfg Config) *Method {
	return &Method{config: cfg}
}

// Restore runs mongorestore against the archive extracted into targetDir.
// Credentials are passed through a temporary config file created outside
// targetDir and removed after the run, so they never reach argv or logs and
// are never written into the extraction directory.
func (m *Method) Restore(ctx context.Context, targetDir string, logger logging.Logger) error {
	logger.Info(ctx, "Stage 3: Restore database")
	start := time.Now()
	defer func() {
		logger.Info(ctx, "Stage 3 completed", logging.Fields{
			"duration": time.Since(start).String(),
		})
	}()

	opts, err := m.sanitizedOptions()
	if err != nil {
		return err
	}

	args, cleanup, err := m.buildArgs(targetDir, opts)
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
		return fmt.Errorf("mongorestore failed: %w", runErr)
	}

	return nil
}

func (m *Method) sanitizedOptions() ([]string, error) {
	if m.config.RestoreOptions == "" {
		return nil, nil
	}

	opts, err := sanitizer.SanitizeOptions(m.config.RestoreOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to sanitize options: %w", err)
	}

	return opts, nil
}

// buildArgs assembles the mongorestore command line. When credentials must
// stay off the command line it writes a 0600 config file in a temporary
// directory outside targetDir and returns a cleanup function that removes it
// again.
func (m *Method) buildArgs(targetDir string, opts []string) ([]string, func() error, error) {
	info := inspectURI(m.config.MongoDB.URI)
	needsFile := m.config.Password != "" || (info.set && info.hasCredentials)

	credsFile, cleanup, err := m.credentialsFile(needsFile, info)
	if err != nil {
		return nil, nil, err
	}

	args := []string{m.config.RestoreBinary}
	args = append(args, m.connectionArgs(info, needsFile, credsFile)...)
	args = append(args, m.serverArgs(info)...)
	args = append(args, "--dir="+targetDir)
	args = append(args, opts...)

	return args, cleanup, nil
}

// credentialsFile writes the temporary mongorestore --config file when
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
	// authenticate: without a username it makes mongorestore attempt SCRAM
	// authentication and fail against servers that do not require auth.
	// Skip it when the URI already carries authSource; mongorestore rejects
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

// writeCredentialsFile writes a mongorestore --config file (YAML keys
// supported by the database tools: uri and password) with mode 0600 in a new
// temporary directory outside targetDir.
func writeCredentialsFile(uri, password string, includeURI bool) (string, error) {
	dir, err := os.MkdirTemp("", "mongorestore-creds")
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
