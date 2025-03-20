package config

import (
	"flag"
	"log"
	"os"
	"sync"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type MariaDB struct {
	Host          string `envconfig:"MARIADB__HOST"`
	Port          int    `envconfig:"MARIADB__PORT"`
	User          string `envconfig:"MARIADB__USER"`
	Password      string `envconfig:"MARIADB__PASSWORD"`
	BackupOptions string `envconfig:"MARIADB__BACKUP_OPTIONS"`
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
		Port:     3306,
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
		if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to load .env: %v", err)
		}
		envconfig.MustProcess("", &config)

		flag.StringVar(&config.MariaDB.Host, "db-host", config.MariaDB.Host, "Database Host")
		flag.IntVar(&config.MariaDB.Port, "db-port", config.MariaDB.Port, "Database Port")
		flag.StringVar(&config.MariaDB.User, "db-user", config.MariaDB.User, "Database User")
		flag.StringVar(&config.MariaDB.Password, "db-password", config.MariaDB.Password, "Database Password")
		flag.StringVar(&config.MariaDB.BackupOptions, "db-backup-options", config.MariaDB.BackupOptions, "Database Backup Options")

		flag.StringVar(&config.Storage.URL, "storage-url", config.Storage.URL, "Storage URL, e.g. s3://my-bucket/my-folder")
		flag.Parse()
	})
	return config
}
