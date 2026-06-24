package retention_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/retention"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

// configOpt configures a retention.Config for testing.
type configOpt func(*retention.Config)

func withMaxCount(n int) configOpt {
	return func(c *retention.Config) { c.MaxCount = n }
}

func withMaxAge(d time.Duration) configOpt {
	return func(c *retention.Config) { c.MaxAge = d }
}

func withKeepDaily(n int) configOpt {
	return func(c *retention.Config) { c.KeepDaily = n }
}

func withKeepWeekly(n int) configOpt {
	return func(c *retention.Config) { c.KeepWeekly = n }
}

func withKeepMonthly(n int) configOpt {
	return func(c *retention.Config) { c.KeepMonthly = n }
}

func withDryRun(b bool) configOpt {
	return func(c *retention.Config) { c.DryRun = b }
}

func withForce(b bool) configOpt {
	return func(c *retention.Config) { c.Force = b }
}

func buildConfig(opts ...configOpt) retention.Config {
	var c retention.Config
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

func newOp(opts ...configOpt) *retention.Operation {
	return retention.NewOperation(buildConfig(opts...), nil, nil, logging.NewTestLogger())
}

func entry(id string, createdAt time.Time, status string) registry.BackupEntry {
	return registry.BackupEntry{
		ID:         id,
		Filename:   "",
		CreatedAt:  createdAt,
		SizeBytes:  0,
		SHA256:     "",
		Encrypted:  false,
		Status:     status,
		Tool:       nil,
		Encryption: nil,
	}
}

func entryFull(
	id, filename string,
	createdAt time.Time,
	sizeBytes int64,
	sha256 string,
) registry.BackupEntry {
	return registry.BackupEntry{
		ID:         id,
		Filename:   filename,
		CreatedAt:  createdAt,
		SizeBytes:  sizeBytes,
		SHA256:     sha256,
		Encrypted:  false,
		Status:     registry.StatusReady,
		Tool:       nil,
		Encryption: nil,
	}
}

func fixedNow() time.Time {
	return time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
}

func sampleBackups(n int) []registry.BackupEntry {
	entries := make([]registry.BackupEntry, n)
	for i := range n {
		t := fixedNow().Add(-time.Duration(i) * time.Hour)
		entries[i] = registry.BackupEntry{
			ID:         fmt.Sprintf("backup-%d", i),
			Filename:   t.Format("2006-01-02-15-04-05") + ".tar.gz",
			CreatedAt:  t,
			SizeBytes:  1000,
			SHA256:     "abc123def456",
			Encrypted:  false,
			Status:     registry.StatusReady,
			Tool:       nil,
			Encryption: nil,
		}
	}
	return entries
}

func ageTestBackups(hoursAgo ...int) []registry.BackupEntry {
	now := time.Now()
	entries := make([]registry.BackupEntry, len(hoursAgo))
	for i, h := range hoursAgo {
		entries[i] = registry.BackupEntry{
			ID:         fmt.Sprintf("age-%d", h),
			Filename:   "",
			CreatedAt:  now.Add(-time.Duration(h) * time.Hour),
			SizeBytes:  0,
			SHA256:     "",
			Encrypted:  false,
			Status:     registry.StatusReady,
			Tool:       nil,
			Encryption: nil,
		}
	}
	return entries
}

func TestConfig_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		cfg  retention.Config
		want bool
	}{
		{name: "all zero", cfg: buildConfig(), want: true},
		{name: "max count set", cfg: buildConfig(withMaxCount(5)), want: false},
		{name: "max age set", cfg: buildConfig(withMaxAge(time.Hour)), want: false},
		{name: "keep daily set", cfg: buildConfig(withKeepDaily(7)), want: false},
		{name: "keep weekly set", cfg: buildConfig(withKeepWeekly(4)), want: false},
		{name: "keep monthly set", cfg: buildConfig(withKeepMonthly(12)), want: false},
		{
			name: "all set",
			cfg: buildConfig(
				withMaxCount(5),
				withMaxAge(time.Hour),
				withKeepDaily(7),
				withKeepWeekly(4),
				withKeepMonthly(12),
			),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.IsEmpty())
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     retention.Config
		wantErr error
	}{
		{name: "empty config", cfg: buildConfig(), wantErr: retention.ErrValidationFailed},
		{name: "max count only", cfg: buildConfig(withMaxCount(5)), wantErr: nil},
		{name: "max age only", cfg: buildConfig(withMaxAge(24 * time.Hour)), wantErr: nil},
		{name: "keep daily only", cfg: buildConfig(withKeepDaily(7)), wantErr: nil},
		{name: "keep weekly only", cfg: buildConfig(withKeepWeekly(4)), wantErr: nil},
		{name: "keep monthly only", cfg: buildConfig(withKeepMonthly(12)), wantErr: nil},
		{
			name:    "negative max count",
			cfg:     buildConfig(withMaxCount(-1), withKeepDaily(7)),
			wantErr: retention.ErrValidationFailed,
		},
		{
			name:    "negative max age",
			cfg:     buildConfig(withMaxAge(-1), withKeepDaily(7)),
			wantErr: retention.ErrValidationFailed,
		},
		{
			name:    "negative keep daily",
			cfg:     buildConfig(withKeepDaily(-1), withMaxCount(5)),
			wantErr: retention.ErrValidationFailed,
		},
		{
			name:    "negative keep weekly",
			cfg:     buildConfig(withKeepWeekly(-1), withMaxCount(5)),
			wantErr: retention.ErrValidationFailed,
		},
		{
			name:    "negative keep monthly",
			cfg:     buildConfig(withKeepMonthly(-1), withMaxCount(5)),
			wantErr: retention.ErrValidationFailed,
		},
		{
			name: "all policies",
			cfg: buildConfig(
				withMaxCount(10),
				withMaxAge(24*time.Hour),
				withKeepDaily(7),
				withKeepWeekly(4),
				withKeepMonthly(12),
			),
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyMaxCountPolicy_UnderLimit(t *testing.T) {
	backups := sampleBackups(3)
	op := newOp(withMaxCount(5))
	keep, remove := op.ApplyMaxCountPolicy(backups)
	assert.Len(t, keep, 3)
	assert.Empty(t, remove)
}

func TestApplyMaxCountPolicy_OverLimit(t *testing.T) {
	backups := sampleBackups(10)
	op := newOp(withMaxCount(3))
	keep, remove := op.ApplyMaxCountPolicy(backups)
	assert.Len(t, keep, 3)
	assert.Len(t, remove, 7)
	assert.Equal(t, "backup-0", keep[0].ID)
	assert.Equal(t, "backup-1", keep[1].ID)
	assert.Equal(t, "backup-2", keep[2].ID)
	assert.Equal(t, "backup-3", remove[0].ID)
}

func TestApplyMaxCountPolicy_Zero(t *testing.T) {
	backups := sampleBackups(5)
	op := newOp(withMaxCount(0))
	keep, remove := op.ApplyMaxCountPolicy(backups)
	assert.Len(t, keep, 5)
	assert.Empty(t, remove)
}

func TestApplyMaxCountPolicy_ExactLimit(t *testing.T) {
	backups := sampleBackups(5)
	op := newOp(withMaxCount(5))
	keep, remove := op.ApplyMaxCountPolicy(backups)
	assert.Len(t, keep, 5)
	assert.Empty(t, remove)
}

func TestApplyMaxCountPolicy_UnsortedInput(t *testing.T) {
	backups := sampleBackups(5)
	backups[0], backups[4] = backups[4], backups[0]
	op := newOp(withMaxCount(3))
	keep, remove := op.ApplyMaxCountPolicy(backups)
	assert.Len(t, keep, 3)
	assert.Len(t, remove, 2)
	assert.Equal(t, "backup-0", keep[0].ID)
}

func TestApplyMaxAgePolicy_AllWithin(t *testing.T) {
	backups := ageTestBackups(1, 2, 3, 4, 5)
	op := newOp(withMaxAge(48 * time.Hour))
	keep, remove := op.ApplyMaxAgePolicy(backups)
	assert.Len(t, keep, 5)
	assert.Empty(t, remove)
}

func TestApplyMaxAgePolicy_SomeExpired(t *testing.T) {
	backups := ageTestBackups(72, 48, 10, 0, 100)
	op := newOp(withMaxAge(24 * time.Hour))
	keep, remove := op.ApplyMaxAgePolicy(backups)
	assert.Len(t, keep, 2)
	assert.Len(t, remove, 3)
}

func TestApplyMaxAgePolicy_AllExpired(t *testing.T) {
	backups := ageTestBackups(72, 48)
	op := newOp(withMaxAge(24 * time.Hour))
	keep, remove := op.ApplyMaxAgePolicy(backups)
	assert.Empty(t, keep)
	assert.Len(t, remove, 2)
}

func TestApplyMaxAgePolicy_Zero(t *testing.T) {
	backups := ageTestBackups(1, 2, 3)
	op := newOp(withMaxAge(0))
	keep, remove := op.ApplyMaxAgePolicy(backups)
	assert.Len(t, keep, 3)
	assert.Empty(t, remove)
}

func TestSelectPeriodBackups_Empty(t *testing.T) {
	op := newOp(withKeepDaily(3))
	result := op.SelectPeriodBackups(
		[]registry.BackupEntry{},
		func(b registry.BackupEntry) string { return b.CreatedAt.Format("2006-01-02") },
		3,
	)
	assert.Empty(t, result)
}

func TestSelectPeriodBackups_MultiplePerPeriod(t *testing.T) {
	now := fixedNow()
	entries := []registry.BackupEntry{
		entry("day1-1", now.Add(-24*time.Hour).Add(-1*time.Hour), registry.StatusReady),
		entry("day1-2", now.Add(-24*time.Hour), registry.StatusReady),
		entry("day2-1", now.Add(-48*time.Hour), registry.StatusReady),
		entry("day2-2", now.Add(-48*time.Hour).Add(-1*time.Hour), registry.StatusReady),
	}
	op := newOp(withKeepDaily(2))
	result := op.SelectPeriodBackups(
		entries,
		func(b registry.BackupEntry) string { return b.CreatedAt.Format("2006-01-02") },
		2,
	)
	assert.Len(t, result, 2)
}

func TestSelectPeriodBackups_CountLimit(t *testing.T) {
	now := fixedNow()
	entries := []registry.BackupEntry{
		entry("day1", now.Add(-24*time.Hour), registry.StatusReady),
		entry("day2", now.Add(-48*time.Hour), registry.StatusReady),
		entry("day3", now.Add(-72*time.Hour), registry.StatusReady),
		entry("day4", now.Add(-96*time.Hour), registry.StatusReady),
		entry("day5", now.Add(-120*time.Hour), registry.StatusReady),
	}
	op := newOp(withKeepDaily(3))
	result := op.SelectPeriodBackups(
		entries,
		func(b registry.BackupEntry) string { return b.CreatedAt.Format("2006-01-02") },
		3,
	)
	assert.Len(t, result, 3)
	assert.Equal(t, entries[0].ID, result[0].ID)
}

func TestSelectPeriodBackups_SinglePeriod(t *testing.T) {
	now := fixedNow()
	entries := []registry.BackupEntry{
		entry("b1", now, registry.StatusReady),
		entry("b2", now.Add(-1*time.Hour), registry.StatusReady),
		entry("b3", now.Add(-2*time.Hour), registry.StatusReady),
	}
	op := newOp(withKeepDaily(1))
	result := op.SelectPeriodBackups(
		entries,
		func(b registry.BackupEntry) string { return b.CreatedAt.Format("2006-01-02") },
		1,
	)
	assert.Len(t, result, 1)
	assert.Equal(t, "b1", result[0].ID)
}

func TestApplyPeriodicPolicies_Daily(t *testing.T) {
	now := fixedNow()
	entries := []registry.BackupEntry{
		entry("d1-late", now.Add(-24*time.Hour).Add(-2*time.Hour), registry.StatusReady),
		entry("d1-early", now.Add(-24*time.Hour), registry.StatusReady),
		entry("d2", now.Add(-48*time.Hour), registry.StatusReady),
		entry("d3", now.Add(-72*time.Hour), registry.StatusReady),
		entry("d4", now.Add(-96*time.Hour), registry.StatusReady),
	}
	op := newOp(withKeepDaily(2))
	keep, remove := op.ApplyPeriodicPolicies(entries)
	assert.Len(t, keep, 2)
	assert.Len(t, remove, 3)
}

func TestApplyPeriodicPolicies_Weekly(t *testing.T) {
	entries := []registry.BackupEntry{
		entry("w1", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), registry.StatusReady),
		entry("w1-late", time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), registry.StatusReady),
		entry("w2", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), registry.StatusReady),
		entry("w3", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), registry.StatusReady),
	}
	op := newOp(withKeepWeekly(2))
	keep, remove := op.ApplyPeriodicPolicies(entries)
	assert.Len(t, keep, 2)
	assert.Len(t, remove, 2)
}

func TestApplyPeriodicPolicies_Monthly(t *testing.T) {
	entries := []registry.BackupEntry{
		entry("m1", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), registry.StatusReady),
		entry("m2", time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC), registry.StatusReady),
		entry("m3", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC), registry.StatusReady),
		entry("m4", time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC), registry.StatusReady),
	}
	op := newOp(withKeepMonthly(2))
	keep, remove := op.ApplyPeriodicPolicies(entries)
	assert.Len(t, keep, 2)
	assert.Len(t, remove, 2)
}

func TestApplyPeriodicPolicies_Combined(t *testing.T) {
	now := fixedNow()
	entries := []registry.BackupEntry{
		entry("latest", now, registry.StatusReady),
		entry("day1", now.Add(-24*time.Hour), registry.StatusReady),
		entry("day2", now.Add(-48*time.Hour), registry.StatusReady),
		entry("day3", now.Add(-72*time.Hour), registry.StatusReady),
		entry("day4", now.Add(-96*time.Hour), registry.StatusReady),
	}
	op := newOp(withKeepDaily(3), withKeepWeekly(2))
	keep, remove := op.ApplyPeriodicPolicies(entries)
	assert.GreaterOrEqual(t, len(keep), 3)
	assert.LessOrEqual(t, len(remove), 2)
}

func TestApplyPeriodicPolicies_AllZero(t *testing.T) {
	entries := sampleBackups(5)
	op := newOp()
	keep, remove := op.ApplyPeriodicPolicies(entries)
	assert.Empty(t, keep)
	assert.Len(t, remove, 5)
}

func TestFilterBackups_OnlyReady(t *testing.T) {
	reg := &registry.Registry{
		Version:   1,
		UpdatedAt: fixedNow(),
		Backups: []registry.BackupEntry{
			entry("ready1", time.Time{}, registry.StatusReady),
			entry("failed1", time.Time{}, registry.StatusFailed),
			entry("deleted1", time.Time{}, registry.StatusDeleted),
			entry("ready2", time.Time{}, registry.StatusReady),
		},
	}
	op := newOp(withMaxCount(5))
	result, err := op.FilterBackups(reg)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "ready1", result[0].ID)
	assert.Equal(t, "ready2", result[1].ID)
}

func TestFilterBackups_Empty(t *testing.T) {
	reg := registry.New()
	op := newOp(withMaxCount(5))
	result, err := op.FilterBackups(reg)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestDeleteBackup_DryRun(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	op := retention.NewOperation(buildConfig(withDryRun(true)), nil, mockBackend, logging.NewTestLogger())
	err := op.DeleteBackup(entryFull("test", "test.tar.gz", fixedNow(), 1000, ""))
	require.NoError(t, err)
	mockBackend.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestDeleteBackup_Success(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockBackend.On("Delete", context.Background(), "test.tar.gz").Return(nil)
	op := retention.NewOperation(buildConfig(), nil, mockBackend, logging.NewTestLogger())
	err := op.DeleteBackup(entryFull("test", "test.tar.gz", fixedNow(), 1000, ""))
	require.NoError(t, err)
	mockBackend.AssertCalled(t, "Delete", context.Background(), "test.tar.gz")
}

func TestDeleteBackup_StorageError(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockBackend.On("Delete", context.Background(), "test.tar.gz").Return(assert.AnError)
	op := retention.NewOperation(buildConfig(), nil, mockBackend, logging.NewTestLogger())
	err := op.DeleteBackup(entryFull("test", "test.tar.gz", fixedNow(), 1000, ""))
	require.Error(t, err)
	assert.ErrorIs(t, err, retention.ErrBackupDeleteFailed)
}

func TestExecuteRetentionActions_DryRun(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	op := retention.NewOperation(buildConfig(withDryRun(true)), nil, mockBackend, logging.NewTestLogger())
	result, err := op.ExecuteRetentionActions(sampleBackups(3))
	require.NoError(t, err)
	assert.Nil(t, result)
	mockBackend.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestExecuteRetentionActions_Success(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockBackend.On("Delete", context.Background(), mock.AnythingOfType("string")).Return(nil)
	op := retention.NewOperation(buildConfig(), nil, mockBackend, logging.NewTestLogger())
	result, err := op.ExecuteRetentionActions(sampleBackups(3))
	require.NoError(t, err)
	assert.Len(t, result, 3)
	mockBackend.AssertNumberOfCalls(t, "Delete", 3)
}

func TestExecuteRetentionActions_ForceMode(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	backups := []registry.BackupEntry{
		entryFull("ok1", "ok1.tar.gz", time.Time{}, 0, ""),
		entryFull("fail", "fail.tar.gz", time.Time{}, 0, ""),
		entryFull("ok2", "ok2.tar.gz", time.Time{}, 0, ""),
	}
	mockBackend.On("Delete", context.Background(), "ok1.tar.gz").Return(nil)
	mockBackend.On("Delete", context.Background(), "fail.tar.gz").Return(assert.AnError)
	mockBackend.On("Delete", context.Background(), "ok2.tar.gz").Return(nil)
	op := retention.NewOperation(buildConfig(withForce(true)), nil, mockBackend, logging.NewTestLogger())
	result, err := op.ExecuteRetentionActions(backups)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "ok1", result[0].ID)
	assert.Equal(t, "ok2", result[1].ID)
}

func TestExecuteRetentionActions_ErrorWithoutForce(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	backups := []registry.BackupEntry{
		entryFull("ok1", "ok1.tar.gz", time.Time{}, 0, ""),
		entryFull("fail", "fail.tar.gz", time.Time{}, 0, ""),
	}
	mockBackend.On("Delete", context.Background(), "ok1.tar.gz").Return(nil)
	mockBackend.On("Delete", context.Background(), "fail.tar.gz").Return(assert.AnError)
	op := retention.NewOperation(buildConfig(), nil, mockBackend, logging.NewTestLogger())
	result, err := op.ExecuteRetentionActions(backups)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestApplyRetentionPolicies_MaxCountOnly(t *testing.T) {
	backups := sampleBackups(10)
	op := newOp(withMaxCount(3))
	toRemove, err := op.ApplyRetentionPolicies(backups)
	require.NoError(t, err)
	assert.Len(t, toRemove, 7)
}

func TestApplyRetentionPolicies_MaxAgeOnly(t *testing.T) {
	backups := ageTestBackups(1, 2, 3, 48, 72)
	op := newOp(withMaxAge(24 * time.Hour))
	toRemove, err := op.ApplyRetentionPolicies(backups)
	require.NoError(t, err)
	assert.Len(t, toRemove, 2)
}

func TestApplyRetentionPolicies_UnionOfKeepSets(t *testing.T) {
	backups := ageTestBackups(0, 1, 2, 3, 4, 5, 6, 48, 72, 96)
	op := newOp(withMaxCount(3), withMaxAge(24*time.Hour))
	toRemove, err := op.ApplyRetentionPolicies(backups)
	require.NoError(t, err)
	assert.Len(t, toRemove, 3)
}

func TestApplyRetentionPolicies_EmptyConfig(t *testing.T) {
	backups := sampleBackups(5)
	op := newOp()
	toRemove, err := op.ApplyRetentionPolicies(backups)
	require.NoError(t, err)
	assert.Len(t, toRemove, 5)
}

func TestLoadRegistry_Success(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	reg := registry.New()
	reg.Backups = sampleBackups(3)
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(reg))

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(buf.Bytes())
		}).
		Return(nil)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withMaxCount(5)), registrySvc, mockBackend, logging.NewTestLogger())

	result, err := op.LoadRegistry()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Backups, 3)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
}

func TestLoadRegistry_NotFound(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).Return(storage.ErrNotFound)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withMaxCount(5)), registrySvc, mockBackend, logging.NewTestLogger())

	result, err := op.LoadRegistry()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Backups)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
}

func TestLoadRegistry_LockError(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(nil, assert.AnError)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withMaxCount(5)), registrySvc, mockBackend, logging.NewTestLogger())

	result, err := op.LoadRegistry()
	require.Error(t, err)
	assert.Nil(t, result)
	require.ErrorIs(t, err, retention.ErrRegistryLoadFailed)
	mockBackend.AssertExpectations(t)
}

func TestUpdateRegistry_Success(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	reg := registry.New()
	reg.Backups = sampleBackups(3)
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(reg))

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(buf.Bytes())
		}).
		Return(nil)
	mockBackend.On("Upload", mock.Anything, ".backup-registry.json", mock.AnythingOfType("*bytes.Buffer")).Return(nil)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withMaxCount(5)), registrySvc, mockBackend, logging.NewTestLogger())

	backupsToRemove := []registry.BackupEntry{
		entryFull("backup-0", "backup-0.tar.gz", fixedNow(), 1000, "abc"),
	}
	err := op.UpdateRegistry(backupsToRemove)
	require.NoError(t, err)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
}

func TestUpdateRegistry_Force(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	reg := registry.New()
	reg.Backups = sampleBackups(3)
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(reg))

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(buf.Bytes())
		}).
		Return(nil)
	mockBackend.On("Upload", mock.Anything, ".backup-registry.json", mock.AnythingOfType("*bytes.Buffer")).
		Return(assert.AnError)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withForce(true)), registrySvc, mockBackend, logging.NewTestLogger())

	backupsToRemove := []registry.BackupEntry{
		entryFull("backup-0", "backup-0.tar.gz", fixedNow(), 1000, "abc"),
	}
	err := op.UpdateRegistry(backupsToRemove)
	require.NoError(t, err)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
}

func TestUpdateRegistry_ErrorWithoutForce(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	reg := registry.New()
	reg.Backups = sampleBackups(3)
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(reg))

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(buf.Bytes())
		}).
		Return(nil)
	mockBackend.On("Upload", mock.Anything, ".backup-registry.json", mock.AnythingOfType("*bytes.Buffer")).
		Return(assert.AnError)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withMaxCount(5)), registrySvc, mockBackend, logging.NewTestLogger())

	backupsToRemove := []registry.BackupEntry{
		entryFull("backup-0", "backup-0.tar.gz", fixedNow(), 1000, "abc"),
	}
	err := op.UpdateRegistry(backupsToRemove)
	require.Error(t, err)
	require.ErrorIs(t, err, retention.ErrRegistryUpdateFailed)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
}

func TestRun_EmptyConfig(t *testing.T) {
	op := newOp()
	err := op.Run(context.Background())
	require.NoError(t, err)
}

func TestRun_LoadRegistryError(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(nil, assert.AnError)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(buildConfig(withMaxCount(5)), registrySvc, mockBackend, logging.NewTestLogger())

	err := op.Run(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, retention.ErrRegistryLoadFailed)
	mockBackend.AssertExpectations(t)
}

func TestRun_FullFlow(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	reg := registry.New()
	reg.Backups = sampleBackups(4)
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(reg))

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(buf.Bytes())
		}).
		Return(nil)
	mockBackend.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil)
	mockBackend.On("Upload", mock.Anything, ".backup-registry.json", mock.AnythingOfType("*bytes.Buffer")).Return(nil)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(
		buildConfig(withMaxCount(2)),
		registrySvc, mockBackend,
		logging.NewTestLogger(),
	)

	err := op.Run(context.Background())
	require.NoError(t, err)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
	mockBackend.AssertNumberOfCalls(t, "Delete", 2)
}

func TestRun_FullFlowDryRun(t *testing.T) {
	mockBackend := &storage.MockBackend{Mock: mock.Mock{}}
	mockLocker := &storage.MockLocker{Mock: mock.Mock{}}

	reg := registry.New()
	reg.Backups = sampleBackups(4)
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(reg))

	mockBackend.On("Lock", mock.Anything, ".backup-registry.json").Return(mockLocker, nil)
	mockBackend.On(
		"Download",
		mock.Anything,
		".backup-registry.json",
		mock.AnythingOfType("*bytes.Buffer"),
	).
		Run(func(args mock.Arguments) {
			w := args.Get(2).(io.Writer)
			_, _ = w.Write(buf.Bytes())
		}).
		Return(nil)
	mockLocker.On("Unlock", mock.Anything).Return(nil)

	registrySvc := registry.NewService(mockBackend)
	op := retention.NewOperation(
		buildConfig(withMaxCount(2), withDryRun(true)),
		registrySvc, mockBackend,
		logging.NewTestLogger(),
	)

	err := op.Run(context.Background())
	require.NoError(t, err)
	mockBackend.AssertExpectations(t)
	mockLocker.AssertExpectations(t)
	mockBackend.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}
