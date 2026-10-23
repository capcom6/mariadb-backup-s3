package mongodb_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/mongodb"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
)

const (
	fakePassword   = "sup3r-s3cret-pw"
	fakeUser       = "backup"
	uriWithSecret  = "mongodb://dbuser:uripass@mongo1:27017/appdb?authSource=appdb"
	uriNoSecret    = "mongodb://mongo1:27017,mongo2:27017/?replicaSet=rs0"
	exitCodeEnvVar = "FAKE_MONGODUMP_EXIT"
)

type fakeMongodump struct {
	argsFile       string
	configPathFile string
	configFileCopy string
}

// installFakeMongodump puts a fake mongodump on PATH that records its argv,
// copies the --config file (if any) while it exists, writes a dump into
// --out, and optionally exits with FAKE_MONGODUMP_EXIT.
func installFakeMongodump(t *testing.T) fakeMongodump {
	t.Helper()

	binDir := t.TempDir()
	workDir := t.TempDir()
	fake := fakeMongodump{
		argsFile:       filepath.Join(workDir, "args"),
		configPathFile: filepath.Join(workDir, "config-path"),
		configFileCopy: filepath.Join(workDir, "config-copy"),
	}

	script := `#!/bin/sh
set -e
: > "$FAKE_MONGODUMP_ARGS"
for arg in "$@"; do
	printf '%s\n' "$arg" >> "$FAKE_MONGODUMP_ARGS"
done
cfg=""
out=""
for arg in "$@"; do
	case "$arg" in
	--config=*) cfg="${arg#--config=}" ;;
	--out=*) out="${arg#--out=}" ;;
	esac
done
if [ -n "$cfg" ]; then
	printf '%s' "$cfg" > "$FAKE_MONGODUMP_CONFIG_PATH"
	cp "$cfg" "$FAKE_MONGODUMP_CONFIG_COPY"
fi
if [ -n "$out" ]; then
	mkdir -p "$out"
	printf 'fake-bson-data' > "$out/dump.bson"
	printf '{}' > "$out/dump.metadata.json"
fi
code="${FAKE_MONGODUMP_EXIT:-0}"
if [ "$code" -ne 0 ]; then
	echo "fake mongodump stderr failure" >&2
fi
exit "$code"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongodump"), []byte(script), 0o755))

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_MONGODUMP_ARGS", fake.argsFile)
	t.Setenv("FAKE_MONGODUMP_CONFIG_PATH", fake.configPathFile)
	t.Setenv("FAKE_MONGODUMP_CONFIG_COPY", fake.configFileCopy)
	t.Setenv(exitCodeEnvVar, "0")

	return fake
}

func readFakeArgs(t *testing.T, fake fakeMongodump) []string {
	t.Helper()

	data, err := os.ReadFile(fake.argsFile)
	require.NoError(t, err)

	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestNew(t *testing.T) {
	m := mongodb.New(mongodb.Config{BackupBinary: "mongodump"})
	assert.NotNil(t, m)
}

func TestMethod_MethodName(t *testing.T) {
	m := mongodb.New(mongodb.Config{BackupBinary: "mongodump"})
	assert.Equal(t, "mongodump", m.MethodName())
	assert.Equal(t, config.BackupMethodMongoDB, m.MethodName())
}

func TestMethod_ImplementsInterface(_ *testing.T) {
	m := mongodb.New(mongodb.Config{BackupBinary: "mongodump"})
	var _ method.Method = m
}

func TestMethod_Prepare_NoOpIdempotent(t *testing.T) {
	m := mongodb.New(mongodb.Config{BackupBinary: "mongodump"})
	logger := logging.NewTestLogger()

	require.NoError(t, m.Prepare(context.Background(), "/tmp/nonexistent", logger))
	require.NoError(t, m.Prepare(context.Background(), "/tmp/nonexistent", logger))
}

func TestBackup_HostPortUserPassword(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "mongo.internal",
		Port:          27018,
		User:          fakeUser,
		Password:      fakePassword,
		BackupOptions: "--gzip --oplog",
		BackupBinary:  "mongodump",
		MongoDB:       config.MongoDB{AuthSource: "admin"},
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.NoError(t, err)

	configPath, pathErr := os.ReadFile(fake.configPathFile)
	require.NoError(t, pathErr)
	credPath := string(configPath)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=mongo.internal",
		"--port=27018",
		"--username=" + fakeUser,
		"--config=" + credPath,
		"--authenticationDatabase=admin",
		"--out=" + tempdir,
		"--gzip",
		"--oplog",
	}, args)

	// Secrets must never appear in argv.
	assert.NotContains(t, strings.Join(args, " "), fakePassword)

	// Credentials live in the config file outside tempdir, readable by mongodump.
	configCopy, copyErr := os.ReadFile(fake.configFileCopy)
	require.NoError(t, copyErr)
	assert.Contains(t, string(configCopy), fakePassword)
	rel, relErr := filepath.Rel(tempdir, credPath)
	require.NoError(t, relErr)
	assert.True(t, strings.HasPrefix(rel, ".."),
		"credentials file must live outside the backup tempdir: %s", credPath)
	assert.Contains(t, filepath.Base(filepath.Dir(credPath)), "mongodump-creds")

	// Config file is removed after the run.
	_, statErr := os.Stat(credPath)
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after Backup")

	// Dump output landed in tempdir; no secret content anywhere under it.
	_, bsonErr := os.Stat(filepath.Join(tempdir, "dump.bson"))
	require.NoError(t, bsonErr)
	walkErr := filepath.WalkDir(tempdir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		assert.NotContains(t, string(data), fakePassword, "secret leaked into %s", path)

		return nil
	})
	require.NoError(t, walkErr)
}

func TestBackup_HostWithoutPortOrUser(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		BackupBinary: "mongodump",
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=localhost",
		"--out=" + tempdir,
	}, args)
}

func TestBackup_URIWithoutCredentials(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		Port:         27017,
		User:         fakeUser,
		BackupBinary: "mongodump",
		MongoDB:      config.MongoDB{URI: uriNoSecret},
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--uri=" + uriNoSecret,
		"--out=" + tempdir,
	}, args)
	// No config file and no host/port/user alongside a URI.
	_, statErr := os.Stat(fake.configPathFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestBackup_URIWithCredentialsKeptOffArgv(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		Port:         27017,
		User:         fakeUser,
		BackupBinary: "mongodump",
		MongoDB: config.MongoDB{
			URI:        uriWithSecret,
			AuthSource: "admin", // must not conflict with authSource inside the URI
		},
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	configPath := string(mustRead(t, fake.configPathFile))
	assert.Equal(t, []string{
		"--config=" + configPath,
		"--out=" + tempdir,
	}, args)

	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "uripass")
	assert.NotContains(t, joined, "--host=")
	assert.NotContains(t, joined, "--port=")
	assert.NotContains(t, joined, "--username=")
	assert.NotContains(t, joined, "--authenticationDatabase=")

	configCopy := string(mustRead(t, fake.configFileCopy))
	assert.Contains(t, configCopy, "uripass")

	_, statErr := os.Stat(configPath)
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after Backup")
}

func TestBackup_TLSFlags(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		Port:         27017,
		User:         fakeUser,
		BackupBinary: "mongodump",
		MongoDB: config.MongoDB{
			AuthSource:            "admin",
			TLS:                   true,
			TLSCAFile:             "/etc/ssl/ca.pem",
			TLSCertificateKeyFile: "/etc/ssl/cert.pem",
		},
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=localhost",
		"--port=27017",
		"--username=" + fakeUser,
		"--authenticationDatabase=admin",
		"--tls",
		"--tlsCAFile=/etc/ssl/ca.pem",
		"--tlsCertificateKeyFile=/etc/ssl/cert.pem",
		"--out=" + tempdir,
	}, args)
}

func TestBackup_DefaultAuthSourceWithoutUserOmitsAuthenticationDatabase(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	// The CLI defaults AuthSource to "admin" for engine mongodb even when no
	// username is configured; without a principal the flag must be omitted or
	// mongodump fails SCRAM handshake against an auth-less server.
	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		Port:         27017,
		BackupBinary: "mongodump",
		MongoDB:      config.MongoDB{AuthSource: "admin"},
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=localhost",
		"--port=27017",
		"--out=" + tempdir,
	}, args)
}

func TestBackup_InvalidBackupOptionsRejectedBeforeExec(t *testing.T) {
	fake := installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		BackupBinary:  "mongodump",
		BackupOptions: "--oplog; rm -rf /tmp",
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.Error(t, err)
	require.ErrorIs(t, err, sanitizer.ErrDangerousCharacters)

	_, statErr := os.Stat(fake.argsFile)
	assert.True(t, os.IsNotExist(statErr), "mongodump must not run with dangerous options")
}

func TestBackup_ExecFailurePropagatesAndCleansUp(t *testing.T) {
	fake := installFakeMongodump(t)
	t.Setenv(exitCodeEnvVar, "7")
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		Port:         27017,
		User:         fakeUser,
		Password:     fakePassword,
		BackupBinary: "mongodump",
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongodump failed")
	assert.Contains(t, err.Error(), "fake mongodump stderr failure")

	// Failure path must also remove the credentials file.
	configPath := string(mustRead(t, fake.configPathFile))
	_, statErr := os.Stat(configPath)
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after failure")
}

func TestBackup_BinaryMissing(t *testing.T) {
	installFakeMongodump(t)
	tempdir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:         "localhost",
		BackupBinary: "mongodump-not-on-path-12345",
	})

	err := m.Backup(context.Background(), tempdir, logging.NewTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}
