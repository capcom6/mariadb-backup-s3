package scheduler_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadState_NewFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	state, err := scheduler.LoadState(path)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Empty(t, state.Status())
}

func TestLoadState_InvalidJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0600))
	_, err := scheduler.LoadState(path)
	require.Error(t, err)
}

func TestState_SaveAndLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")

	s1, err := scheduler.LoadState(path)
	require.NoError(t, err)
	s1.RecordRun("test-job", scheduler.StatusSuccess, nil, 5*time.Minute)
	require.NoError(t, s1.Save())

	s2, err := scheduler.LoadState(path)
	require.NoError(t, err)
	states := s2.Status()
	require.Len(t, states, 1)
	assert.Equal(t, "test-job", states[0].Name)
	assert.Equal(t, scheduler.StatusSuccess, states[0].LastRunStatus)
	assert.Equal(t, int64(1), states[0].RunCount)
	assert.Equal(t, int64(0), states[0].FailureCount)
}

func TestState_RecordRun(t *testing.T) {
	t.Parallel()

	state, err := scheduler.LoadState(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, err)

	state.RecordRun("job1", scheduler.StatusSuccess, nil, 2*time.Minute)
	state.RecordRun("job2", scheduler.StatusFailed, assert.AnError, 30*time.Second)
	state.RecordRun("job1", scheduler.StatusSuccess, nil, 3*time.Minute)

	states := state.Status()
	require.Len(t, states, 2)

	for _, st := range states {
		switch st.Name {
		case "job1":
			assert.Equal(t, scheduler.StatusSuccess, st.LastRunStatus)
			assert.Equal(t, int64(2), st.RunCount)
			assert.Equal(t, int64(0), st.FailureCount)
			assert.Empty(t, st.LastRunError)
		case "job2":
			assert.Equal(t, scheduler.StatusFailed, st.LastRunStatus)
			assert.Equal(t, int64(1), st.RunCount)
			assert.Equal(t, int64(1), st.FailureCount)
			assert.NotEmpty(t, st.LastRunError)
		}
	}
}

func TestState_RecordNextRun(t *testing.T) {
	t.Parallel()

	state, err := scheduler.LoadState(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, err)

	next := time.Now().UTC().Add(1 * time.Hour)
	state.RecordNextRun("job1", next)
	state.RecordRun("job1", scheduler.StatusSuccess, nil, 1*time.Minute)

	states := state.Status()
	require.Len(t, states, 1)
	assert.WithinDuration(t, next, states[0].NextRunAt, time.Second)
}

func TestState_Close(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")

	state, err := scheduler.LoadState(path)
	require.NoError(t, err)
	state.RecordRun("job", scheduler.StatusSuccess, nil, 1*time.Minute)
	require.NoError(t, state.Close())

	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestState_EmptyStatus(t *testing.T) {
	t.Parallel()

	state, err := scheduler.LoadState(filepath.Join(t.TempDir(), "empty.json"))
	require.NoError(t, err)
	assert.Empty(t, state.Status())
}
