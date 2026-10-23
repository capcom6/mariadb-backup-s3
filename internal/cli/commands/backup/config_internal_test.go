package backup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"

	"github.com/capcom6/mariadb-backup-s3/internal/cli/flags"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

func runParseDatabase(t *testing.T, args ...string) config.Database {
	t.Helper()

	var db config.Database
	cmd := &cli.Command{
		Name:  "test",
		Flags: flags.Database(),
		Action: func(_ context.Context, c *cli.Command) error {
			db = parseDatabase(c)
			return nil
		},
	}

	err := cmd.Run(context.Background(), append([]string{"test"}, args...))
	assert.NoError(t, err)

	return db
}

func TestParseDatabase_MongoDBDefaults(t *testing.T) {
	db := runParseDatabase(t, "--db-engine=mongodb")

	assert.Equal(t, config.EngineMongoDB, db.Engine)
	assert.Equal(t, 27017, db.Port)
	assert.Equal(t, config.BackupMethodMongoDB, db.BackupMethod)
	assert.Equal(t, "mongodump", db.BackupBinary)
	assert.Equal(t, "mongorestore", db.RestoreBinary)
	assert.Equal(t, "admin", db.MongoDB.AuthSource)
}

func TestParseDatabase_MongoDBFlags(t *testing.T) {
	db := runParseDatabase(
		t,
		"--db-engine=mongodb",
		"--mongo-uri=mongodb://mongo1:27017",
		"--mongo-auth-source=backupdb",
		"--mongo-tls",
		"--mongo-tls-ca-file=/etc/ssl/ca.pem",
		"--mongo-tls-certificate-key-file=/etc/ssl/cert.pem",
	)

	assert.Equal(t, "mongodb://mongo1:27017", db.MongoDB.URI)
	assert.Equal(t, "backupdb", db.MongoDB.AuthSource)
	assert.True(t, db.MongoDB.TLS)
	assert.Equal(t, "/etc/ssl/ca.pem", db.MongoDB.TLSCAFile)
	assert.Equal(t, "/etc/ssl/cert.pem", db.MongoDB.TLSCertificateKeyFile)
}

func TestParseDatabase_MongoDBURIFromEnv(t *testing.T) {
	t.Setenv("MONGODB__URI", "mongodb://env-host:27017")

	db := runParseDatabase(t, "--db-engine=mongodb")

	assert.Equal(t, "mongodb://env-host:27017", db.MongoDB.URI)
}

func TestParseDatabase_MariaDBHasNoMongoAuthDefault(t *testing.T) {
	db := runParseDatabase(t)

	assert.Equal(t, config.EngineMariaDB, db.Engine)
	assert.Empty(t, db.MongoDB.AuthSource)
	assert.Empty(t, db.MongoDB.URI)
}
