package restore

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/restore"
	"github.com/urfave/cli/v3"
)

// defaultMongoAuthSource is the MongoDB authentication database used when
// --mongo-auth-source is not provided.
const defaultMongoAuthSource = "admin"

func parseConfig(cmd *cli.Command, engine string) (restore.Config, error) {
	cfg := restore.DefaultConfig()

	cfg.Storage.URL = cmd.String("storage-url")
	cfg.Encryption.EncryptionKey = cmd.String("encryption-key")
	cfg.TargetDir = cmd.String("target-dir")
	cfg.Database = parseDatabase(cmd, engine)

	if err := cfg.Validate(); err != nil {
		return restore.Config{}, fmt.Errorf("failed to parse restore configuration: %w", err)
	}

	return cfg, nil
}

func parseDatabase(cmd *cli.Command, engine string) config.Database {
	if engine == "" {
		engine = config.EngineMariaDB
	}

	db := config.DefaultDatabase(engine)

	if value := cmd.String("db-host"); value != "" {
		db.Host = value
	}
	if value := cmd.Int("db-port"); value != 0 {
		db.Port = value
	}
	if value := cmd.String("db-user"); value != "" {
		db.User = value
	}
	db.Password = cmd.String("db-password")

	if value := cmd.String("db-restore-options"); value != "" {
		db.RestoreOptions = value
	}
	if value := cmd.String("db-restore-binary"); value != "" {
		db.RestoreBinary = value
	}

	applyMongoDBFlags(cmd, &db)

	return db
}

func applyMongoDBFlags(cmd *cli.Command, db *config.Database) {
	db.MongoDB = config.MongoDB{
		URI:                   cmd.String("mongo-uri"),
		AuthSource:            cmd.String("mongo-auth-source"),
		TLS:                   cmd.Bool("mongo-tls"),
		TLSCAFile:             cmd.String("mongo-tls-ca-file"),
		TLSCertificateKeyFile: cmd.String("mongo-tls-certificate-key-file"),
	}

	if db.Engine == config.EngineMongoDB && db.MongoDB.AuthSource == "" {
		db.MongoDB.AuthSource = defaultMongoAuthSource
	}
}
