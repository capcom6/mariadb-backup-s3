package scheduler_test

import (
	"testing"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
	"github.com/stretchr/testify/assert"
)

func TestBuildMariaDB_LogicalMethod_DefaultBinary(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		MariaDB: &repository.MariaDB{
			BackupMethod: config.BackupMethodLogical,
		},
	}

	cfg := scheduler.BuildMariaDB(job)
	assert.Equal(t, config.BackupMethodLogical, cfg.BackupMethod)
	assert.Equal(t, "mariadb-dump", cfg.BackupBinary)
}

func TestBuildMariaDB_PhysicalMethod_DefaultBinary(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		MariaDB: &repository.MariaDB{
			BackupMethod: config.BackupMethodPhysical,
		},
	}

	cfg := scheduler.BuildMariaDB(job)
	assert.Equal(t, config.BackupMethodPhysical, cfg.BackupMethod)
	assert.Equal(t, "mariadb-backup", cfg.BackupBinary)
}

func TestBuildMariaDB_ExplicitBinary_Preserved(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		MariaDB: &repository.MariaDB{
			BackupMethod: config.BackupMethodLogical,
			BackupBinary: "/opt/custom/mariadb-dump",
		},
	}

	cfg := scheduler.BuildMariaDB(job)
	assert.Equal(t, "/opt/custom/mariadb-dump", cfg.BackupBinary)
}

func TestBuildMariaDB_NilMariaDB_UsesDefaults(t *testing.T) {
	t.Parallel()

	job := repository.Job{}

	cfg := scheduler.BuildMariaDB(job)
	assert.Equal(t, config.DefaultMariaDB().BackupBinary, cfg.BackupBinary)
	assert.Equal(t, config.DefaultMariaDB().BackupMethod, cfg.BackupMethod)
}
