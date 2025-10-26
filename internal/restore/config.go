package restore

import "github.com/capcom6/mariadb-backup-s3/internal/config"

type Config struct {
	Storage    config.Storage
	Encryption config.Encryption

	TargetDir string
}

func DefaultConfig() Config {
	return Config{
		Storage: config.Storage{
			URL: "",
		},
		Encryption: config.Encryption{
			EncryptionKey: "",
		},

		TargetDir: "",
	}
}
