package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
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

func (m *mockMethod) ArtifactExtension() string {
	return method.ArtifactTarGz
}

func (m *mockMethod) getCallOrder() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.callOrder))
	copy(result, m.callOrder)
	return result
}

// mockStreamMethod is a backup method that streams its output.
type mockStreamMethod struct {
	content string
	err     error
}

func (m *mockStreamMethod) Backup(_ context.Context, _ string, _ logging.Logger) error {
	return errors.New("unexpected call to Backup")
}

func (m *mockStreamMethod) Prepare(_ context.Context, _ string, _ logging.Logger) error {
	return nil
}

func (m *mockStreamMethod) MethodName() string {
	return "mariadb-dump"
}

func (m *mockStreamMethod) ArtifactExtension() string {
	return "sql.gz"
}

func (m *mockStreamMethod) Stream(_ context.Context, w io.Writer, _ logging.Logger) error {
	if _, err := io.WriteString(w, m.content); err != nil {
		return err
	}

	return m.err
}

func writeTestFile(t *testing.T, tempdir string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(tempdir, "test.sql"), []byte("SELECT 1;"), 0o644)
	require.NoError(t, err)
}

// capturedUpload holds the content of a single upload.
type capturedUpload struct {
	data []byte
	err  error
}

// newCapturingMockStorage returns a storage mock that keeps the uploaded
// content, keyed by file name.
func newCapturingMockStorage(t *testing.T) (*storage.MockBackend, *registry.Service, map[string]capturedUpload) {
	t.Helper()

	captured := make(map[string]capturedUpload)

	backend := &storage.MockBackend{}
	backend.On("Upload", mock.Anything, mock.AnythingOfType("string"), mock.Anything).
		Return(nil).Run(func(args mock.Arguments) {
		filename := args.String(1)
		r := args.Get(2).(io.Reader)
		data, err := io.ReadAll(r)
		captured[filename] = capturedUpload{data: data, err: err}
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

	return backend, regSvc, captured
}

func TestOperation_Run_CallsBackupThenPrepare(t *testing.T) {
	method := new(mockMethod)
	method.On("Backup", mock.AnythingOfType("string")).Return(nil).Run(func(args mock.Arguments) {
		writeTestFile(t, args.String(0))
	})
	method.On("Prepare", mock.AnythingOfType("string")).Return(nil)

	backend, regSvc, captured := newCapturingMockStorage(t)

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

	// the file-based backup is stored as a .tar.gz archive
	artifactName, _, found := findUpload(captured, ".tar.gz")
	assert.True(t, found, "no .tar.gz object was uploaded")
	assert.NotEmpty(t, artifactName)
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

func TestOperation_Run_Streaming(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	backend, regSvc, captured := newCapturingMockStorage(t)

	streamMethod := &mockStreamMethod{content: "-- mariadb-backup-s3: database=mydb\nSELECT 1;\n"}

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		MariaDB: config.MariaDB{
			BackupBinary: "mariadb-dump",
			BackupMethod: config.BackupMethodLogical,
			User:         "root",
		},
		Storage: config.Storage{URL: "file:///tmp/test"},
		Stream:  true,
	}

	op := &Operation{
		config:      cfg,
		method:      streamMethod,
		streamer:    streamMethod,
		registrySvc: regSvc,
		storage:     backend,
		logger:      logger,
	}

	require.NoError(t, op.Run(context.Background()))

	// the backup is stored as a single .sql.gz object
	artifactName, artifact, found := findUpload(captured, ".sql.gz")
	require.True(t, found, "no .sql.gz object was uploaded")
	require.NoError(t, artifact.err)

	// the object is named after the start of the backup
	startedAt, err := time.ParseInLocation("2006-01-02-15-04-05", strings.TrimSuffix(artifactName, ".sql.gz"), time.UTC)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), startedAt, time.Minute)

	// the object holds the gzip of the streamed dump
	reader, err := gzip.NewReader(bytes.NewReader(artifact.data))
	require.NoError(t, err)
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, streamMethod.content, string(decompressed))

	// nothing was written to the local disk
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestOperation_Run_StreamingFails_NoObjectUploaded(t *testing.T) {
	storageDir := t.TempDir()

	u, err := url.Parse("file://" + storageDir)
	require.NoError(t, err)
	backend, err := storage.New(u)
	require.NoError(t, err)

	streamMethod := &mockStreamMethod{
		content: "-- mariadb-backup-s3: database=mydb\nSELECT 1;\n",
		err:     errors.New("dump failed"),
	}

	logger := logging.NewTestLogger()
	cfg := Config{
		Version: "test",
		MariaDB: config.MariaDB{
			BackupBinary: "mariadb-dump",
			BackupMethod: config.BackupMethodLogical,
			User:         "root",
		},
		Storage: config.Storage{URL: "file://" + storageDir},
		Stream:  true,
	}

	op := &Operation{
		config:      cfg,
		method:      streamMethod,
		streamer:    streamMethod,
		registrySvc: registry.NewService(backend),
		storage:     backend,
		logger:      logger,
	}

	err = op.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dump failed")

	// neither a partial backup nor a registry entry was stored
	entries, err := os.ReadDir(storageDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestNewOperation_WiresStreamerOnlyForStreamedLogicalBackup(t *testing.T) {
	logger := logging.NewTestLogger()

	logicalConfig := Config{
		Version: "test",
		MariaDB: config.MariaDB{
			BackupBinary: "mariadb-dump",
			BackupMethod: config.BackupMethodLogical,
			User:         "root",
		},
		Storage: config.Storage{URL: "file:///tmp/test"},
		Stream:  true,
	}
	op := NewOperation(logicalConfig, nil, nil, logger)
	require.NotNil(t, op.streamer)
	assert.Equal(t, "sql.gz", op.method.ArtifactExtension())

	logicalConfig.Stream = false
	op = NewOperation(logicalConfig, nil, nil, logger)
	assert.Nil(t, op.streamer)
	assert.Equal(t, "tar.gz", op.method.ArtifactExtension())

	physicalConfig := logicalConfig
	physicalConfig.MariaDB.BackupMethod = config.BackupMethodPhysical
	physicalConfig.MariaDB.BackupBinary = "mariadb-backup"
	op = NewOperation(physicalConfig, nil, nil, logger)
	assert.Nil(t, op.streamer)
	assert.Equal(t, "tar.gz", op.method.ArtifactExtension())
}

// findUpload returns the uploaded object with the given suffix.
func findUpload(captured map[string]capturedUpload, suffix string) (string, capturedUpload, bool) {
	for name, upload := range captured {
		if strings.HasSuffix(name, suffix) {
			return name, upload, true
		}
	}

	return "", capturedUpload{}, false
}
