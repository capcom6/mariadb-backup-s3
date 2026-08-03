package repository_test

import (
	"testing"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  repository.Config
		wantErr string
	}{
		{
			name: "valid backup job",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "nightly-backup",
						Schedule: "0 2 * * *",
						Command:  "backup",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid retention job",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "weekly-retention",
						Schedule: "0 3 * * 0",
						Command:  "retention",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
						Retention: &repository.Retention{
							MaxCount: 7,
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "multiple jobs",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "backup",
						Schedule: "0 2 * * *",
						Command:  "backup",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
					{
						Name:     "retention",
						Schedule: "0 3 * * 0",
						Command:  "retention",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
				},
			},
			wantErr: "",
		},
		{
			name:    "empty jobs",
			config:  repository.Config{},
			wantErr: "at least one job must be defined",
		},
		{
			name: "missing job name",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Schedule: "0 2 * * *",
						Command:  "backup",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
				},
			},
			wantErr: "name is required",
		},
		{
			name: "missing schedule",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:    "test",
						Command: "backup",
						Storage: repository.Storage{URL: "s3://bucket/path"},
					},
				},
			},
			wantErr: "schedule is required",
		},
		{
			name: "invalid command",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "test",
						Schedule: "0 2 * * *",
						Command:  "unknown",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
				},
			},
			wantErr: `unsupported command "unknown"`,
		},
		{
			name: "missing storage URL",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "test",
						Schedule: "0 2 * * *",
						Command:  "backup",
						Storage:  repository.Storage{},
					},
				},
			},
			wantErr: "storage.url is required",
		},
		{
			name: "duplicate job names",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "dup",
						Schedule: "0 2 * * *",
						Command:  "backup",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
					{
						Name:     "dup",
						Schedule: "0 3 * * *",
						Command:  "retention",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
					},
				},
			},
			wantErr: `duplicate job name: "dup"`,
		},
		{
			name: "disabled job passes validation",
			config: repository.Config{
				Jobs: []repository.Job{
					{
						Name:     "disabled-job",
						Schedule: "0 2 * * *",
						Command:  "backup",
						Storage:  repository.Storage{URL: "s3://bucket/path"},
						Enabled:  boolPtr(false),
					},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestJob_IsEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{name: "nil defaults to true", enabled: nil, want: true},
		{name: "explicit true", enabled: boolPtr(true), want: true},
		{name: "explicit false", enabled: boolPtr(false), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			job := repository.Job{Enabled: tt.enabled}
			assert.Equal(t, tt.want, job.IsEnabled())
		})
	}
}

func TestJob_GetTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		job      repository.Job
		wantSecs int64
	}{
		{name: "backup default", job: repository.Job{Command: "backup"}, wantSecs: 4 * 3600},
		{name: "retention default", job: repository.Job{Command: "retention"}, wantSecs: 30 * 60},
		{
			name:     "custom timeout",
			job:      repository.Job{Command: "backup", Timeout: repository.Duration(2 * time.Hour)},
			wantSecs: 2 * 3600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.job.GetTimeout()
			assert.Equal(t, tt.wantSecs, int64(got.Seconds()))
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestConfig_YAML_BackupMethod(t *testing.T) {
	t.Parallel()

	yamlData := `
jobs:
  - name: logical-backup
    schedule: "0 2 * * *"
    command: backup
    storage:
      url: "s3://bucket/path"
    mariadb:
      backup_method: mariadb-dump
      host: localhost
      user: root
`
	var cfg repository.Config
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Jobs, 1)
	require.NotNil(t, cfg.Jobs[0].MariaDB)
	assert.Equal(t, "mariadb-dump", cfg.Jobs[0].MariaDB.BackupMethod)
	assert.Equal(t, "localhost", cfg.Jobs[0].MariaDB.Host)
}

func TestConfig_YAML_BackupMethodOmitted(t *testing.T) {
	t.Parallel()

	yamlData := `
jobs:
  - name: physical-backup
    schedule: "0 2 * * *"
    command: backup
    storage:
      url: "s3://bucket/path"
    mariadb:
      host: localhost
`
	var cfg repository.Config
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Jobs, 1)
	require.NotNil(t, cfg.Jobs[0].MariaDB)
	assert.Empty(t, cfg.Jobs[0].MariaDB.BackupMethod)
}
