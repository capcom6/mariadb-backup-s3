package registry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

func fixedNow() time.Time {
	return time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
}

func entry(id, filename string, createdAt time.Time, status string) registry.BackupEntry {
	return registry.BackupEntry{
		ID:         id,
		Filename:   filename,
		CreatedAt:  createdAt,
		SizeBytes:  1024,
		SHA256:     "abcdef1234567890",
		Status:     status,
		Encrypted:  false,
		Encryption: nil,
		Tool:       nil,
	}
}

func buildService(opts ...registry.Option) (*registry.Service, *storage.MockBackend) {
	backend := &storage.MockBackend{Mock: mock.Mock{}}
	svc := registry.NewService(backend, opts...)
	return svc, backend
}

func mockLock(backend *storage.MockBackend) *storage.MockLocker {
	locker := &storage.MockLocker{Mock: mock.Mock{}}
	locker.On("Unlock", mock.Anything).Return(nil)
	backend.On("Lock", mock.Anything, ".backup-registry.json").Return(locker, nil)
	return locker
}

func validRegistryJSON() string {
	return `{"version":1,"updated_at":"2024-01-15T10:30:00Z","backups":[]}`
}

func TestNew(t *testing.T) {
	reg := registry.New()
	assert.Equal(t, 1, reg.Version)
	assert.False(t, reg.UpdatedAt.IsZero())
	assert.Empty(t, reg.Backups)
}

func TestNewBackupEntry(t *testing.T) {
	now := fixedNow()
	entry := registry.NewBackupEntry(
		"backup.tar.gz", now, 2048, "abcdef1234567890",
		registry.StatusReady, false, nil, nil,
	)
	assert.Contains(t, entry.ID, "2024-01-15-10-30-00-")
	assert.Equal(t, "backup.tar.gz", entry.Filename)
	assert.Equal(t, int64(2048), entry.SizeBytes)
	assert.Equal(t, "abcdef1234567890", entry.SHA256)
	assert.Equal(t, registry.StatusReady, entry.Status)
	assert.False(t, entry.Encrypted)
}

func TestRegistryValidate_Valid(t *testing.T) {
	reg := registry.New()
	reg.UpdatedAt = fixedNow()
	assert.NoError(t, reg.Validate())
}

func TestRegistryValidate_InvalidVersion(t *testing.T) {
	reg := registry.New()
	reg.UpdatedAt = fixedNow()
	reg.Version = 99
	assert.ErrorIs(t, reg.Validate(), registry.ErrValidationFailed)
}

func TestRegistryValidate_ZeroUpdatedAt(t *testing.T) {
	reg := registry.New()
	reg.UpdatedAt = time.Time{}
	assert.ErrorIs(t, reg.Validate(), registry.ErrValidationFailed)
}

func TestRegistryValidate_DuplicateID(t *testing.T) {
	now := fixedNow()
	reg := registry.New()
	reg.UpdatedAt = now
	reg.Backups = []registry.BackupEntry{
		entry("dup", "a.tar.gz", now, registry.StatusReady),
		entry("dup", "b.tar.gz", now, registry.StatusReady),
	}
	assert.ErrorIs(t, reg.Validate(), registry.ErrValidationFailed)
}

func TestRegistryValidate_DuplicateFilename(t *testing.T) {
	now := fixedNow()
	reg := registry.New()
	reg.UpdatedAt = now
	reg.Backups = []registry.BackupEntry{
		entry("a", "same.tar.gz", now, registry.StatusReady),
		entry("b", "same.tar.gz", now, registry.StatusReady),
	}
	assert.ErrorIs(t, reg.Validate(), registry.ErrValidationFailed)
}

func TestBackupEntryValidate_Valid(t *testing.T) {
	now := fixedNow()
	e := entry("id1", "backup.tar.gz", now, registry.StatusReady)
	assert.NoError(t, e.Validate())
}

func TestBackupEntryValidate_AllStatuses(t *testing.T) {
	now := fixedNow()
	for _, status := range []string{registry.StatusReady, registry.StatusFailed, registry.StatusDeleted} {
		e := entry("id1", "backup.tar.gz", now, status)
		assert.NoError(t, e.Validate(), "status %q should be valid", status)
	}
}

func TestBackupEntryValidate_EmptyID(t *testing.T) {
	now := fixedNow()
	e := entry("", "backup.tar.gz", now, registry.StatusReady)
	assert.ErrorIs(t, e.Validate(), registry.ErrValidationFailed)
}

func TestBackupEntryValidate_EmptyFilename(t *testing.T) {
	now := fixedNow()
	e := entry("id1", "", now, registry.StatusReady)
	assert.ErrorIs(t, e.Validate(), registry.ErrValidationFailed)
}

func TestBackupEntryValidate_ZeroCreatedAt(t *testing.T) {
	e := entry("id1", "backup.tar.gz", time.Time{}, registry.StatusReady)
	assert.ErrorIs(t, e.Validate(), registry.ErrValidationFailed)
}

func TestBackupEntryValidate_NegativeSize(t *testing.T) {
	e := registry.BackupEntry{
		ID:         "id1",
		Filename:   "backup.tar.gz",
		CreatedAt:  fixedNow(),
		SizeBytes:  -1,
		SHA256:     "abcdef1234567890",
		Status:     registry.StatusReady,
		Encrypted:  false,
		Encryption: nil,
		Tool:       nil,
	}
	assert.ErrorIs(t, e.Validate(), registry.ErrValidationFailed)
}

func TestBackupEntryValidate_EmptySHA256(t *testing.T) {
	now := fixedNow()
	e := registry.BackupEntry{
		ID:         "id1",
		Filename:   "backup.tar.gz",
		CreatedAt:  now,
		SizeBytes:  1024,
		SHA256:     "",
		Status:     registry.StatusReady,
		Encrypted:  false,
		Encryption: nil,
		Tool:       nil,
	}
	assert.ErrorIs(t, e.Validate(), registry.ErrValidationFailed)
}

func TestBackupEntryValidate_InvalidStatus(t *testing.T) {
	now := fixedNow()
	e := registry.BackupEntry{
		ID:         "id1",
		Filename:   "backup.tar.gz",
		CreatedAt:  now,
		SizeBytes:  1024,
		SHA256:     "abcdef1234567890",
		Status:     "unknown",
		Encrypted:  false,
		Encryption: nil,
		Tool:       nil,
	}
	assert.ErrorIs(t, e.Validate(), registry.ErrValidationFailed)
}

func TestRegistrySortByCreatedAtDesc(t *testing.T) {
	now := fixedNow()
	reg := registry.New()
	reg.Backups = []registry.BackupEntry{
		entry("old", "a.tar.gz", now.Add(-2*time.Hour), registry.StatusReady),
		entry("new", "b.tar.gz", now, registry.StatusReady),
		entry("mid", "c.tar.gz", now.Add(-1*time.Hour), registry.StatusReady),
	}
	reg.SortByCreatedAtDesc()
	assert.Equal(t, "new", reg.Backups[0].ID)
	assert.Equal(t, "mid", reg.Backups[1].ID)
	assert.Equal(t, "old", reg.Backups[2].ID)
}

func TestBackupTimeFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     time.Time
		ok       bool
	}{
		{"2024-01-15-10-30-00.tar.gz", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), true},
		{"2024-01-15-10-30-00.tar.gz.enc", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), true},
		{"2024-01-15-10-30-00.enc", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), true},
		{"invalid.tar.gz", time.Time{}, false},
		{"", time.Time{}, false},
	}
	for _, tt := range tests {
		got, ok := registry.BackupTimeFromFilename(tt.filename)
		assert.Equal(t, tt.ok, ok, "filename %q", tt.filename)
		if tt.ok {
			assert.True(t, got.Equal(tt.want), "filename %q: got %v, want %v", tt.filename, got, tt.want)
		}
	}
}

func TestNewService(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		svc, _ := buildService()
		assert.NotNil(t, svc)
	})

	t.Run("with recovery", func(t *testing.T) {
		svc, _ := buildService(registry.WithRecovery())
		assert.NotNil(t, svc)
	})
}

func TestServiceLoad_HappyPath(t *testing.T) {
	svc, backend := buildService()
	locker := mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write([]byte(validRegistryJSON()))
		})

	reg, err := svc.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, 1, reg.Version)

	locker.AssertExpectations(t)
	backend.AssertExpectations(t)
}

func TestServiceLoad_NotFound_NoRecovery(t *testing.T) {
	svc, backend := buildService()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).Return(storage.ErrNotFound)

	reg, err := svc.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Empty(t, reg.Backups)

	backend.AssertExpectations(t)
}

func TestServiceLoad_NotFound_WithRecovery(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).Return(storage.ErrNotFound)
	backend.On("List", mock.Anything).Return([]string{"2024-01-15-10-30-00.tar.gz"}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	reg, err := svc.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Len(t, reg.Backups, 1)
	assert.Equal(t, "2024-01-15-10-30-00", reg.Backups[0].ID)

	backend.AssertExpectations(t)
}

func TestServiceLoad_NotFound_WithRecovery_NoFiles(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).Return(storage.ErrNotFound)
	backend.On("List", mock.Anything).Return([]string{}, nil)

	reg, err := svc.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Empty(t, reg.Backups)

	backend.AssertExpectations(t)
}

func TestServiceLoad_DownloadError(t *testing.T) {
	svc, backend := buildService()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).Return(errors.New("network error"))

	_, err := svc.Load(context.Background())
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceLoad_ParseError(t *testing.T) {
	svc, backend := buildService()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write([]byte(`invalid json`))
		})

	_, err := svc.Load(context.Background())
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceAppend_HappyPath(t *testing.T) {
	svc, backend := buildService()
	now := fixedNow()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write([]byte(validRegistryJSON()))
		})
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	entry := entry("new1", "backup.tar.gz", now, registry.StatusReady)
	reg, err := svc.Append(context.Background(), entry)
	require.NoError(t, err)
	require.Len(t, reg.Backups, 1)
	assert.Equal(t, "new1", reg.Backups[0].ID)

	backend.AssertExpectations(t)
}

func TestServiceAppend_DownloadError(t *testing.T) {
	svc, backend := buildService()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).Return(errors.New("network error"))

	entry := entry("new1", "backup.tar.gz", fixedNow(), registry.StatusReady)
	_, err := svc.Append(context.Background(), entry)
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceAppend_SaveError(t *testing.T) {
	svc, backend := buildService()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write([]byte(validRegistryJSON()))
		})
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(errors.New("upload failed"))

	entry := entry("new1", "backup.tar.gz", fixedNow(), registry.StatusReady)
	_, err := svc.Append(context.Background(), entry)
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceMarkDeleted_HappyPath(t *testing.T) {
	svc, backend := buildService()
	now := fixedNow()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			reg := registry.New()
			reg.UpdatedAt = now
			reg.Backups = []registry.BackupEntry{
				entry("del1", "backup.tar.gz", now, registry.StatusReady),
			}
			b, _ := json.Marshal(reg)
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(b)
		})
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	err := svc.MarkDeleted(context.Background(), "del1")
	require.NoError(t, err)

	backend.AssertExpectations(t)
}

func TestServiceMarkDeleted_IDNotFound(t *testing.T) {
	svc, backend := buildService()
	now := fixedNow()
	mockLock(backend)

	backend.On("Download", mock.Anything, ".backup-registry.json", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			reg := registry.New()
			reg.UpdatedAt = now
			reg.Backups = []registry.BackupEntry{
				entry("existing", "backup.tar.gz", now, registry.StatusReady),
			}
			b, _ := json.Marshal(reg)
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(b)
		})
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	err := svc.MarkDeleted(context.Background(), "nonexistent")
	require.NoError(t, err)

	backend.AssertExpectations(t)
}

func TestServiceParse_ValidJSON(t *testing.T) {
	svc, _ := buildService()
	r := strings.NewReader(validRegistryJSON())
	reg, err := svc.Parse(r)
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Version)
}

func TestServiceParse_InvalidJSON(t *testing.T) {
	svc, _ := buildService()
	r := strings.NewReader("not json")
	_, err := svc.Parse(r)
	require.Error(t, err)
}

func TestServiceParse_InvalidVersion(t *testing.T) {
	svc, _ := buildService()
	r := strings.NewReader(`{"version":99,"updated_at":"2024-01-15T10:30:00Z","backups":[]}`)
	_, err := svc.Parse(r)
	assert.ErrorIs(t, err, registry.ErrValidationFailed)
}

func TestServiceParse_MissingRequiredField(t *testing.T) {
	svc, _ := buildService()
	r := strings.NewReader(`{"version":1,"updated_at":"2024-01-15T10:30:00Z","backups":[{"id":""}]}`)
	_, err := svc.Parse(r)
	require.Error(t, err)
}

func TestServiceSave_InvalidRegistry(t *testing.T) {
	svc, backend := buildService()

	reg := &registry.Registry{Version: 99, UpdatedAt: fixedNow(), Backups: nil}
	err := svc.Save(context.Background(), reg)
	require.Error(t, err)
	backend.AssertNotCalled(t, "Upload", mock.Anything, mock.Anything, mock.Anything)
}

func TestServiceSave_StorageError(t *testing.T) {
	svc, backend := buildService()
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(errors.New("upload failed"))

	now := fixedNow()
	reg := registry.New()
	reg.UpdatedAt = now
	err := svc.Save(context.Background(), reg)
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithoutRecovery(t *testing.T) {
	svc, backend := buildService()
	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	assert.Empty(t, reg.Backups)
	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_ListsFiles(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{
		"2024-01-15-10-30-00.tar.gz",
		"2024-01-14-10-30-00.tar.gz.enc",
	}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	require.Len(t, reg.Backups, 2)
	assert.Equal(t, registry.StatusReady, reg.Backups[0].Status)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_NoFiles(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{}, nil)

	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	assert.Empty(t, reg.Backups)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_IgnoresRegistryFile(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{
		".backup-registry.json",
		"2024-01-15-10-30-00.tar.gz",
	}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	require.Len(t, reg.Backups, 1)
	assert.Equal(t, "2024-01-15-10-30-00", reg.Backups[0].ID)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_IgnoresDirectories(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{
		"subfolder/",
		"2024-01-15-10-30-00.tar.gz",
	}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	require.Len(t, reg.Backups, 1)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_IgnoresInvalidFilenames(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{
		"readme.txt",
		"2024-01-15-10-30-00.tar.gz",
	}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	require.Len(t, reg.Backups, 1)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_SetsEncryptedFlag(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{
		"2024-01-15-10-30-00.tar.gz.enc",
	}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(nil)

	reg, err := svc.Rebuild(context.Background())
	require.NoError(t, err)
	require.Len(t, reg.Backups, 1)
	assert.True(t, reg.Backups[0].Encrypted)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_ListError(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return(nil, errors.New("list failed"))

	_, err := svc.Rebuild(context.Background())
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceRebuild_WithRecovery_SaveError(t *testing.T) {
	svc, backend := buildService(registry.WithRecovery())

	backend.On("List", mock.Anything).Return([]string{
		"2024-01-15-10-30-00.tar.gz",
	}, nil)
	backend.On("Upload", mock.Anything, ".backup-registry.json", mock.Anything).Return(errors.New("upload failed"))

	_, err := svc.Rebuild(context.Background())
	require.Error(t, err)

	backend.AssertExpectations(t)
}

func TestServiceParse_EmptyReader(t *testing.T) {
	svc, _ := buildService()
	r := bytes.NewReader(nil)
	_, err := svc.Parse(r)
	require.Error(t, err)
}

func TestNewBackupEntry_WithEncryption(t *testing.T) {
	now := fixedNow()
	enc := &registry.EncryptionMetadata{Algorithm: "AES-256-GCM"}
	tool := &registry.ToolMetadata{Name: "test-tool", Version: "1.0.0"}
	e := registry.NewBackupEntry(
		"backup.tar.gz", now, 1024, "abcdef1234567890",
		registry.StatusReady, true, enc, tool,
	)
	assert.True(t, e.Encrypted)
	require.NotNil(t, e.Encryption)
	assert.Equal(t, "AES-256-GCM", e.Encryption.Algorithm)
	require.NotNil(t, e.Tool)
	assert.Equal(t, "test-tool", e.Tool.Name)
}

func TestRegistryValidate_EmptyBackupsOK(t *testing.T) {
	reg := registry.New()
	reg.UpdatedAt = fixedNow()
	assert.NoError(t, reg.Validate())
}

func TestBackupEntryValidate_ZeroSizeOK(t *testing.T) {
	e := registry.BackupEntry{
		ID:         "id1",
		Filename:   "backup.tar.gz",
		CreatedAt:  fixedNow(),
		SizeBytes:  0,
		SHA256:     "abcdef1234567890",
		Status:     registry.StatusReady,
		Encrypted:  false,
		Encryption: nil,
		Tool:       nil,
	}
	assert.NoError(t, e.Validate())
}
