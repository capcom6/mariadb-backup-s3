package backup

import "github.com/capcom6/mariadb-backup-s3/internal/config"

type BackupLimitsConfig struct {
	MaxCount int
}

type Backup struct {
	Limits BackupLimitsConfig
}

type Config struct {
	MariaDB    config.MariaDB
	Storage    config.Storage
	Backup     Backup
	Encryption config.Encryption
}

func DefaultConfig() Config {
	return Config{
		MariaDB: config.MariaDB{
			Host:          "localhost",
			Port:          3306,
			User:          "root",
			Password:      "",
			BackupOptions: "",
		},
		Storage: config.Storage{
			URL: "",
		},
		Backup: Backup{
			Limits: BackupLimitsConfig{MaxCount: 0},
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
	}
}
