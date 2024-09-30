package config

import (
	"flag"
	"sync"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type MariaDB struct {
	User     string `envconfig:"MARIADB__USER"`
	Password string `envconfig:"MARIADB__PASSWORD"`
}

type Storage struct {
	URL string `envconfig:"STORAGE__URL"`
}

type BackupLimits struct {
	MaxCount int `envconfig:"BACKUP__LIMITS__MAX_COUNT"`
}

type Backup struct {
	Limits BackupLimits
}

type Config struct {
	MariaDB MariaDB
	Storage Storage
	Backup  Backup
}

var onceLoader sync.Once
var config = Config{
	MariaDB: MariaDB{
		User:     "root",
		Password: "",
	},
	Storage: Storage{
		URL: "",
	},
	Backup: Backup{
		Limits: BackupLimits{MaxCount: 0},
	},
}

func Load() Config {
	onceLoader.Do(func() {
		godotenv.Load()
		envconfig.MustProcess("", &config)

		flag.StringVar(&config.MariaDB.User, "db-user", config.MariaDB.User, "Database User")
		flag.StringVar(&config.MariaDB.Password, "db-password", config.MariaDB.Password, "Database Password")

		flag.StringVar(&config.Storage.URL, "storage-url", config.Storage.URL, "Storage URL, e.g. s3://my-bucket/my-folder")
		flag.Parse()
	})
	return config
}
