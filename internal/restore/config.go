package restore

import "github.com/capcom6/mariadb-backup-s3/internal/config"

type RestoreConfig struct {
	TargetDir string
}

type Config struct {
	Storage    config.Storage
	Encryption config.Encryption
	Restore    RestoreConfig
}

func DefaultConfig() Config {
	return Config{
		Storage: config.Storage{
			URL: "",
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},
		Restore: RestoreConfig{
			TargetDir: "",
		},
	}
}
