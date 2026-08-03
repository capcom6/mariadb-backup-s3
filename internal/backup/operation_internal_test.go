package backup

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

func newMockStorage(t *testing.T) (*storage.MockBackend, *registry.Service) {
	t.Helper()
	backend := &storage.MockBackend{}
	backend.On("Upload", mock.Anything, mock.AnythingOfType("string"), mock.Anything).
		Return(nil).Run(func(args mock.Arguments) {
		r := args.Get(2).(io.Reader)
		_, _ = io.Copy(io.Discard, r)
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
	return backend, regSvc
}

func TestOperation_Run_CallsBackupThenPrepare(t *testing.T) {
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(nil).Run(func(args mock.Arguments) {
		writeTestFile(t, args.String(0))
	})
	method.On("Prepare", mock.AnythingOfType("string")).Return(nil)

	backend, regSvc := newMockStorage(t)

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		MariaDB: config.MariaDB{
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
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(assert.AnError)

	backend := &storage.MockBackend{}

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		MariaDB: config.MariaDB{
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
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(nil).Run(func(args mock.Arguments) {
		writeTestFile(t, args.String(0))
	})
	method.On("Prepare", mock.AnythingOfType("string")).Return(assert.AnError)

	backend := &storage.MockBackend{}

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		MariaDB: config.MariaDB{
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
