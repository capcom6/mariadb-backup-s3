package mongodb_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/restore/method"
	"github.com/capcom6/mariadb-backup-s3/internal/restore/method/mongodb"
	"github.com/capcom6/mariadb-backup-s3/internal/sanitizer"
)

const (
	fakePassword   = "sup3r-s3cret-pw"
	fakeUser       = "restore"
	uriWithSecret  = "mongodb://dbuser:uripass@mongo1:27017/appdb?authSource=appdb"
	uriNoSecret    = "mongodb://mongo1:27017,mongo2:27017/?replicaSet=rs0"
	exitCodeEnvVar = "FAKE_MONGORESTORE_EXIT"
)

type fakeMongorestore struct {
	argsFile       string
	configPathFile string
	configFileCopy string
}

// installFakeMongorestore puts a fake mongorestore on PATH that records its
// argv, copies the --config file (if any) while it exists, and optionally
// exits with FAKE_MONGORESTORE_EXIT.
func installFakeMongorestore(t *testing.T) fakeMongorestore {
	t.Helper()

	binDir := t.TempDir()
	workDir := t.TempDir()
	fake := fakeMongorestore{
		argsFile:       filepath.Join(workDir, "args"),
		configPathFile: filepath.Join(workDir, "config-path"),
		configFileCopy: filepath.Join(workDir, "config-copy"),
	}

	script := `#!/bin/sh
set -e
: > "$FAKE_MONGORESTORE_ARGS"
for arg in "$@"; do
	printf '%s\n' "$arg" >> "$FAKE_MONGORESTORE_ARGS"
done
cfg=""
for arg in "$@"; do
	case "$arg" in
	--config=*) cfg="${arg#--config=}" ;;
	esac
done
if [ -n "$cfg" ]; then
	printf '%s' "$cfg" > "$FAKE_MONGORESTORE_CONFIG_PATH"
	cp "$cfg" "$FAKE_MONGORESTORE_CONFIG_COPY"
fi
code="${FAKE_MONGORESTORE_EXIT:-0}"
if [ "$code" -ne 0 ]; then
	echo "fake mongorestore stderr failure" >&2
fi
exit "$code"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongorestore"), []byte(script), 0o755))

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_MONGORESTORE_ARGS", fake.argsFile)
	t.Setenv("FAKE_MONGORESTORE_CONFIG_PATH", fake.configPathFile)
	t.Setenv("FAKE_MONGORESTORE_CONFIG_COPY", fake.configFileCopy)
	t.Setenv(exitCodeEnvVar, "0")

	return fake
}

func readFakeArgs(t *testing.T, fake fakeMongorestore) []string {
	t.Helper()

	data, err := os.ReadFile(fake.argsFile)
	require.NoError(t, err)

	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestNew(t *testing.T) {
	m := mongodb.New(mongodb.Config{RestoreBinary: "mongorestore"})
	assert.NotNil(t, m)
}

func TestMethod_ImplementsInterface(_ *testing.T) {
	m := mongodb.New(mongodb.Config{RestoreBinary: "mongorestore"})
	var _ method.Method = m
}

func TestRestore_HostPortUserPassword(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:           "mongo.internal",
		Port:           27018,
		User:           fakeUser,
		Password:       fakePassword,
		RestoreOptions: "--drop --oplogReplay",
		RestoreBinary:  "mongorestore",
		MongoDB:        config.MongoDB{AuthSource: "admin"},
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
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
		"--dir=" + targetDir,
		"--drop",
		"--oplogReplay",
	}, args)

	// Secrets must never appear in argv.
	assert.NotContains(t, strings.Join(args, " "), fakePassword)

	// Credentials live in the config file outside targetDir, readable by mongorestore.
	configCopy, copyErr := os.ReadFile(fake.configFileCopy)
	require.NoError(t, copyErr)
	assert.Contains(t, string(configCopy), fakePassword)
	rel, relErr := filepath.Rel(targetDir, credPath)
	require.NoError(t, relErr)
	assert.True(t, strings.HasPrefix(rel, ".."),
		"credentials file must live outside the target dir: %s", credPath)
	assert.Contains(t, filepath.Base(filepath.Dir(credPath)), "mongorestore-creds")

	// Config file is removed after the run.
	_, statErr := os.Stat(credPath)
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after Restore")
}

func TestRestore_HostWithoutPortOrUser(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		RestoreBinary: "mongorestore",
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=localhost",
		"--dir=" + targetDir,
	}, args)
	// No password means no credentials file.
	_, statErr := os.Stat(fake.configPathFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRestore_URIWithoutCredentials(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		Port:          27017,
		User:          fakeUser,
		RestoreBinary: "mongorestore",
		MongoDB:       config.MongoDB{URI: uriNoSecret},
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--uri=" + uriNoSecret,
		"--dir=" + targetDir,
	}, args)
	_, statErr := os.Stat(fake.configPathFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRestore_URIWithCredentialsKeptOffArgv(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		Port:          27017,
		User:          fakeUser,
		RestoreBinary: "mongorestore",
		MongoDB: config.MongoDB{
			URI:        uriWithSecret,
			AuthSource: "admin", // must not conflict with authSource inside the URI
		},
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	configPath := string(mustRead(t, fake.configPathFile))
	assert.Equal(t, []string{
		"--config=" + configPath,
		"--dir=" + targetDir,
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
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after Restore")
}

func TestRestore_TLSFlags(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		Port:          27017,
		User:          fakeUser,
		RestoreBinary: "mongorestore",
		MongoDB: config.MongoDB{
			AuthSource:            "admin",
			TLS:                   true,
			TLSCAFile:             "/etc/ssl/ca.pem",
			TLSCertificateKeyFile: "/etc/ssl/cert.pem",
		},
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
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
		"--dir=" + targetDir,
	}, args)
}

func TestRestore_RestoreOptionsPassedThrough(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:           "localhost",
		RestoreBinary:  "mongorestore",
		RestoreOptions: "--drop --nsInclude=appdb.* --gzip",
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=localhost",
		"--dir=" + targetDir,
		"--drop",
		"--nsInclude=appdb.*",
		"--gzip",
	}, args)
}

func TestRestore_DefaultAuthSourceWithoutUserOmitsAuthenticationDatabase(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	// The CLI defaults AuthSource to "admin" for engine mongodb even when no
	// username is configured; without a principal the flag must be omitted.
	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		Port:          27017,
		RestoreBinary: "mongorestore",
		MongoDB:       config.MongoDB{AuthSource: "admin"},
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.NoError(t, err)

	args := readFakeArgs(t, fake)
	assert.Equal(t, []string{
		"--host=localhost",
		"--port=27017",
		"--dir=" + targetDir,
	}, args)
}

func TestRestore_InvalidRestoreOptionsRejectedBeforeExec(t *testing.T) {
	fake := installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:           "localhost",
		RestoreBinary:  "mongorestore",
		RestoreOptions: "--drop; rm -rf /tmp",
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.Error(t, err)
	require.ErrorIs(t, err, sanitizer.ErrDangerousCharacters)

	_, statErr := os.Stat(fake.argsFile)
	assert.True(t, os.IsNotExist(statErr), "mongorestore must not run with dangerous options")
}

func TestRestore_ExecFailurePropagatesAndCleansUp(t *testing.T) {
	fake := installFakeMongorestore(t)
	t.Setenv(exitCodeEnvVar, "7")
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		Port:          27017,
		User:          fakeUser,
		Password:      fakePassword,
		RestoreBinary: "mongorestore",
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongorestore failed")
	assert.Contains(t, err.Error(), "fake mongorestore stderr failure")

	// Failure path must also remove the credentials file.
	configPath := string(mustRead(t, fake.configPathFile))
	_, statErr := os.Stat(configPath)
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after failure")
}

func TestRestore_BinaryMissing(t *testing.T) {
	installFakeMongorestore(t)
	targetDir := t.TempDir()

	m := mongodb.New(mongodb.Config{
		Host:          "localhost",
		RestoreBinary: "mongorestore-not-on-path-12345",
	})

	err := m.Restore(context.Background(), targetDir, logging.NewTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}
