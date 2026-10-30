package backup

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

type mockMethod struct {
	mock.Mock

	callOrder []string
	mu        sync.Mutex
}

func (m *mockMethod) Backup(_ context.Context, tempdir string, _ logging.Logger) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callOrder = append(m.callOrder, "Backup")
	args := m.Called(tempdir)
	return args.Error(0)
}

func (m *mockMethod) Prepare(_ context.Context, tempdir string, _ logging.Logger) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callOrder = append(m.callOrder, "Prepare")
	args := m.Called(tempdir)
	return args.Error(0)
}

func (m *mockMethod) MethodName() string {
	return "test-method"
}

func (m *mockMethod) getCallOrder() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.callOrder))
	copy(result, m.callOrder)
	return result
}

func writeTestFile(t *testing.T, tempdir string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(tempdir, "test.sql"), []byte("SELECT 1;"), 0o644)
	require.NoError(t, err)
}

type capturedUpload struct {
	name string
	body []byte
}

func newMockStorage(t *testing.T) (*storage.MockBackend, *registry.Service, *[]capturedUpload) {
	t.Helper()
	backend := &storage.MockBackend{}
	uploads := &[]capturedUpload{}
	backend.On("Upload", mock.Anything, mock.AnythingOfType("string"), mock.Anything).
		Return(nil).Run(func(args mock.Arguments) {
		r := args.Get(2).(io.Reader)
		body, err := io.ReadAll(r)
		require.NoError(t, err)
		*uploads = append(*uploads, capturedUpload{name: args.String(1), body: body})
	})
	locker := &storage.MockLocker{}
	locker.On("Unlock", mock.Anything).Return(nil)
	backend.On("Lock", mock.Anything, ".backup-registry.json").Return(locker, nil)
	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).Run(func(args mock.Arguments) {
		w := args.Get(2).(io.Writer)
		reg := registry.New()
		b, _ := json.Marshal(reg)
		_, _ = w.Write(b)
	})

	regSvc := registry.NewService(backend)
	return backend, regSvc, uploads
}

func TestOperation_Run_CallsBackupThenPrepare(t *testing.T) {
	t.Parallel()
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(nil).Run(func(args mock.Arguments) {
		writeTestFile(t, args.String(0))
	})
	method.On("Prepare", mock.AnythingOfType("string")).Return(nil)

	backend, regSvc, _ := newMockStorage(t)

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		Database: config.Database{
			BackupBinary: "mariadb-backup",
			BackupMethod: config.BackupMethodPhysical,
			User:         "root",
		},
		Storage: config.Storage{URL: "file:///tmp/test"},
	}

	op := &Operation{
		config:      cfg,
		method:      method,
		registrySvc: regSvc,
		storage:     backend,
		logger:      logger,
	}

	err := op.Run(context.Background())
	require.NoError(t, err)

	order := method.getCallOrder()
	assert.Equal(t, []string{"Backup", "Prepare"}, order)
	method.AssertExpectations(t)
}

func TestOperation_Run_BackupFails(t *testing.T) {
	t.Parallel()
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(assert.AnError)

	backend := &storage.MockBackend{}

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		Database: config.Database{
			BackupBinary: "mariadb-backup",
			BackupMethod: config.BackupMethodPhysical,
			User:         "root",
		},
		Storage: config.Storage{URL: "file:///tmp/test"},
	}

	op := &Operation{
		config:      cfg,
		method:      method,
		registrySvc: nil,
		storage:     backend,
		logger:      logger,
	}

	err := op.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to backup")

	method.AssertNotCalled(t, "Prepare", mock.Anything)
}

func TestOperation_Run_PrepareFails(t *testing.T) {
	t.Parallel()
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(nil).Run(func(args mock.Arguments) {
		writeTestFile(t, args.String(0))
	})
	method.On("Prepare", mock.AnythingOfType("string")).Return(assert.AnError)

	backend := &storage.MockBackend{}

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		Database: config.Database{
			BackupBinary: "mariadb-backup",
			BackupMethod: config.BackupMethodPhysical,
			User:         "root",
		},
		Storage: config.Storage{URL: "file:///tmp/test"},
	}

	op := &Operation{
		config:      cfg,
		method:      method,
		registrySvc: nil,
		storage:     backend,
		logger:      logger,
	}

	err := op.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to prepare")
}

func TestNewOperation_MongoDBReturnsMongodumpMethod(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Database: config.Database{
			Engine:       config.EngineMongoDB,
			BackupMethod: config.BackupMethodMongoDB,
		},
	}

	op, err := NewOperation(cfg, nil, nil, logging.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, op)
	assert.Equal(t, config.BackupMethodMongoDB, op.method.MethodName())
	assert.Equal(t, "mongodump", op.method.MethodName())
}

func TestNewOperation_MongoDBIncompatibleBackupMethod(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Database: config.Database{
			Engine:       config.EngineMongoDB,
			BackupMethod: config.BackupMethodLogical,
		},
	}

	op, err := NewOperation(cfg, nil, nil, logging.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, op)
	assert.ErrorIs(t, err, config.ErrInvalidBackupMethod)
}

func TestNewOperation_UnknownEngineFails(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Database: config.Database{
			Engine: "postgres",
		},
	}

	op, err := NewOperation(cfg, nil, nil, logging.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, op)
	assert.ErrorIs(t, err, config.ErrInvalidEngine)
}

func TestNewOperation_EmptyEngineDefaultsToMariaDBPhysical(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Database: config.Database{},
	}

	op, err := NewOperation(cfg, nil, nil, logging.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, op)
	assert.Equal(t, config.BackupMethodPhysical, op.method.MethodName())
}

//nolint:paralleltest // fake-binary helper mutates process env via t.Setenv; Go forbids t.Parallel in tests that call it
func TestOperation_Run_MongoDBRegistryMethod(t *testing.T) {
	fake := installFakeMongodump(t)

	backend, regSvc, uploads := newMockStorage(t)

	cfg := Config{
		Version: "test",
		Database: config.Database{
			Engine:       config.EngineMongoDB,
			BackupMethod: config.BackupMethodMongoDB,
			BackupBinary: "mongodump",
			User:         "backup",
			Password:     "registry-test-secret",
		},
		Storage: config.Storage{URL: "file:///tmp/test"},
	}

	op, err := NewOperation(cfg, regSvc, backend, logging.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, op.Run(context.Background()))

	// mongodump must have been executed with a credentials file outside tempdir.
	configPath := string(mustReadFile(t, fake.configPathFile))
	_, statErr := os.Stat(configPath)
	assert.True(t, os.IsNotExist(statErr), "credentials file must be removed after the run")

	// Registry upload must carry tool.method=mongodump.
	var registryBody []byte
	var archiveUploaded bool
	for _, u := range *uploads {
		switch {
		case u.name == ".backup-registry.json":
			registryBody = u.body
		case strings.HasSuffix(u.name, ".tar.gz"):
			archiveUploaded = true
		}
	}
	assert.True(t, archiveUploaded, "backup archive must be uploaded")

	require.NotEmpty(t, registryBody)
	var reg registry.Registry
	require.NoError(t, json.Unmarshal(registryBody, &reg))
	require.NotEmpty(t, reg.Backups)

	last := reg.Backups[len(reg.Backups)-1]
	require.NotNil(t, last.Tool)
	assert.Equal(t, "mongodump", last.Tool.Method)
	assert.Equal(t, "mariadb-backup-s3", last.Tool.Name)
}

// installFakeMongodump puts a fake mongodump on PATH that records its argv,
// copies the --config file (if any) while it exists, and writes a dump into
// --out.
func installFakeMongodump(t *testing.T) fakeMongodump {
	t.Helper()

	binDir := t.TempDir()
	workDir := t.TempDir()
	fake := fakeMongodump{
		argsFile:       filepath.Join(workDir, "args"),
		configPathFile: filepath.Join(workDir, "config-path"),
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
fi
if [ -n "$out" ]; then
	mkdir -p "$out"
	printf 'fake-bson-data' > "$out/dump.bson"
fi
exit 0
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mongodump"), []byte(script), 0o755))

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_MONGODUMP_ARGS", fake.argsFile)
	t.Setenv("FAKE_MONGODUMP_CONFIG_PATH", fake.configPathFile)

	return fake
}

type fakeMongodump struct {
	argsFile       string
	configPathFile string
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}
