package backup

import "github.com/capcom6/mariadb-backup-s3/internal/config"

type LimitsConfig struct {
	MaxCount int
}

type Backup struct {
	Limits LimitsConfig
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
			Port:          3306, //nolint:mnd // default port
			User:          "root",
			Password:      "",
			BackupOptions: "",
		},
		Storage: config.Storage{
			URL: "",
		},
		Backup: Backup{
			Limits: LimitsConfig{MaxCount: 0},
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
	}
}
