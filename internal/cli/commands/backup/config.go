package backup

import (
	"fmt"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/urfave/cli/v3"
)

func parseConfig(cmd *cli.Command) (backup.Config, error) {
	cfg := backup.DefaultConfig()

	cfg.Version = cmd.Root().Version
	cfg.Database = parseDatabase(cmd)

	// Configure storage
	cfg.Storage.URL = cmd.String("storage-url")

	// Configure encryption
	cfg.Encryption.EncryptionKey = cmd.String("encryption-key")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return backup.Config{}, fmt.Errorf("failed to parse backup configuration: %w", err)
	}

	return cfg, nil
}

func parseDatabase(cmd *cli.Command) config.Database {
	engine := cmd.String("db-engine")
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

	if value := cmd.String("db-backup-options"); value != "" {
		db.BackupOptions = value
	}
	if value := cmd.String("db-restore-options"); value != "" {
		db.RestoreOptions = value
	}
	if value := cmd.String("backup-method"); value != "" {
		db.BackupMethod = value
	}

	applyBinaryDefaults(cmd, &db)
	applyMongoDBFlags(cmd, &db)

	return db.Normalize()
}

func applyBinaryDefaults(cmd *cli.Command, db *config.Database) {
	if value := cmd.String("db-backup-binary"); value != "" {
		db.BackupBinary = value
	} else {
		db.BackupBinary = config.DefaultBackupBinary(db.Engine, db.BackupMethod)
	}

	if value := cmd.String("db-client-binary"); value != "" {
		db.ClientBinary = value
	} else {
		db.ClientBinary = config.DefaultDatabase(db.Engine).ClientBinary
	}

	if value := cmd.String("db-restore-binary"); value != "" {
		db.RestoreBinary = value
	} else {
		db.RestoreBinary = config.DefaultDatabase(db.Engine).RestoreBinary
	}
}

func applyMongoDBFlags(cmd *cli.Command, db *config.Database) {
	db.MongoDB = config.MongoDB{
		URI:                   cmd.String("mongo-uri"),
		AuthSource:            cmd.String("mongo-auth-source"),
		TLS:                   cmd.Bool("mongo-tls"),
		TLSCAFile:             cmd.String("mongo-tls-ca-file"),
		TLSCertificateKeyFile: cmd.String("mongo-tls-certificate-key-file"),
	}
}
