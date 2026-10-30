package scheduler_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, config.DefaultDatabase(config.EngineMariaDB).BackupBinary, cfg.BackupBinary)
	assert.Equal(t, config.DefaultDatabase(config.EngineMariaDB).BackupMethod, cfg.BackupMethod)
}

func TestBuildDatabase_NoSections_Defaults(t *testing.T) {
	t.Parallel()

	cfg := scheduler.BuildDatabase(repository.Job{})

	assert.Equal(t, config.DefaultDatabase(config.EngineMariaDB), cfg)
}

func TestBuildDatabase_LegacyMariaDB_EngineMariaDB(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		MariaDB: &repository.MariaDB{
			Host:         "db-1",
			BackupMethod: config.BackupMethodLogical,
		},
	}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, config.EngineMariaDB, cfg.Engine)
	assert.Equal(t, "db-1", cfg.Host)
	assert.Equal(t, config.BackupMethodLogical, cfg.BackupMethod)
	assert.Equal(t, "mariadb-dump", cfg.BackupBinary)
	assert.Equal(t, "mariadb", cfg.ClientBinary)
	assert.Equal(t, 3306, cfg.Port)
}

func TestBuildDatabase_DatabaseSection_MongoDBFull(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		Database: &repository.Database{
			Engine:                config.EngineMongoDB,
			Host:                  "mongo-1",
			Port:                  27017,
			User:                  "backup",
			Password:              "secret",
			BackupMethod:          config.BackupMethodMongoDB,
			BackupBinary:          "/opt/mongodb/bin/mongodump",
			ClientBinary:          "/opt/mongodb/bin/mongosh",
			BackupOptions:         "--oplog",
			URI:                   "mongodb://mongo-1:27017",
			AuthSource:            "admin",
			TLS:                   true,
			TLSCAFile:             "/etc/ssl/ca.pem",
			TLSCertificateKeyFile: "/etc/ssl/client.pem",
		},
	}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, config.EngineMongoDB, cfg.Engine)
	assert.Equal(t, "mongo-1", cfg.Host)
	assert.Equal(t, 27017, cfg.Port)
	assert.Equal(t, "backup", cfg.User)
	assert.Equal(t, "secret", cfg.Password)
	assert.Equal(t, config.BackupMethodMongoDB, cfg.BackupMethod)
	assert.Equal(t, "/opt/mongodb/bin/mongodump", cfg.BackupBinary)
	assert.Equal(t, "/opt/mongodb/bin/mongosh", cfg.ClientBinary)
	assert.Equal(t, "--oplog", cfg.BackupOptions)
	assert.Equal(t, config.MongoDB{
		URI:                   "mongodb://mongo-1:27017",
		AuthSource:            "admin",
		TLS:                   true,
		TLSCAFile:             "/etc/ssl/ca.pem",
		TLSCertificateKeyFile: "/etc/ssl/client.pem",
	}, cfg.MongoDB)
}

func TestBuildDatabase_DatabaseSection_MongoDBDefaults(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		Database: &repository.Database{Engine: config.EngineMongoDB},
	}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, config.EngineMongoDB, cfg.Engine)
	assert.Equal(t, 27017, cfg.Port)
	assert.Equal(t, config.BackupMethodMongoDB, cfg.BackupMethod)
	assert.Equal(t, "mongodump", cfg.BackupBinary)
	assert.Empty(t, cfg.MongoDB.URI)
}

func TestBuildDatabase_DatabaseSection_MongoDBAuthSourceDefault(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		Database: &repository.Database{
			Engine: config.EngineMongoDB,
			User:   "backup",
		},
	}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, "admin", cfg.MongoDB.AuthSource)
}

func TestBuildDatabase_DatabaseSection_MongoDBExplicitAuthSourcePreserved(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		Database: &repository.Database{
			Engine:     config.EngineMongoDB,
			User:       "backup",
			AuthSource: "authdb",
		},
	}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, "authdb", cfg.MongoDB.AuthSource)
}

func TestBuildDatabase_MariaDBAuthSourceUnaffected(t *testing.T) {
	t.Parallel()

	legacy := scheduler.BuildDatabase(repository.Job{
		MariaDB: &repository.MariaDB{User: "backup"},
	})
	section := scheduler.BuildDatabase(repository.Job{
		Database: &repository.Database{Engine: config.EngineMariaDB, User: "backup"},
	})

	assert.Empty(t, legacy.MongoDB.AuthSource)
	assert.Empty(t, section.MongoDB.AuthSource)
}

func TestBuildDatabase_DatabaseSection_MariaDBLogical(t *testing.T) {
	t.Parallel()

	job := repository.Job{
		Database: &repository.Database{
			Engine:       config.EngineMariaDB,
			Host:         "db-1",
			BackupMethod: config.BackupMethodLogical,
		},
	}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, config.EngineMariaDB, cfg.Engine)
	assert.Equal(t, "db-1", cfg.Host)
	assert.Equal(t, 3306, cfg.Port)
	assert.Equal(t, config.BackupMethodLogical, cfg.BackupMethod)
	assert.Equal(t, "mariadb-dump", cfg.BackupBinary)
	assert.Equal(t, "mariadb", cfg.ClientBinary)
	assert.Empty(t, cfg.MongoDB.URI)
}

func TestBuildDatabase_DatabaseSection_EmptyDefaultsToMariaDB(t *testing.T) {
	t.Parallel()

	job := repository.Job{Database: &repository.Database{}}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, config.EngineMariaDB, cfg.Engine)
	assert.Equal(t, config.DefaultDatabase(config.EngineMariaDB), cfg)
}

func TestBuildDatabase_DatabaseSection_InvalidEngineRejectedAtValidate(t *testing.T) {
	t.Parallel()

	job := repository.Job{Database: &repository.Database{Engine: "postgres"}}

	cfg := scheduler.BuildDatabase(job)

	assert.Equal(t, "postgres", cfg.Engine)
	err := cfg.Validate()
	require.ErrorIs(t, err, config.ErrInvalidEngine)
}

// TestNew_WebhookSectionAbsent_Succeeds is the regression test for the E2E
// crash: a config with jobs but no top-level webhook: section must construct
// the scheduler (webhooks.NewService must not fail on an empty URL).
func TestNew_WebhookSectionAbsent_Succeeds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	cfgPath := filepath.Join(dir, "scheduler.yaml")

	yamlConfig := "state_file: " + statePath + "\n" +
		"jobs:\n" +
		"  - name: mongo-backup\n" +
		"    schedule: \"@daily\"\n" +
		"    command: backup\n" +
		"    database:\n" +
		"      engine: mongodb\n" +
		"    storage:\n" +
		"      url: file://" + dir + "/backups/\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(yamlConfig), 0o600))

	sched, err := scheduler.New(scheduler.Config{
		ConfigPath: cfgPath,
		StatePath:  statePath,
	}, logging.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, sched)
	require.NoError(t, sched.Close())
}
