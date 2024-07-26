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

type Config struct {
	MariaDB MariaDB
	Storage Storage
}

var onceLoader sync.Once
var config Config

func Load() Config {
	onceLoader.Do(func() {
		godotenv.Load()
		envconfig.Process("", &config)

		flag.StringVar(&config.MariaDB.User, "db-user", config.MariaDB.User, "Database User")
		flag.StringVar(&config.MariaDB.Password, "db-password", config.MariaDB.Password, "Database Password")

		flag.StringVar(&config.Storage.URL, "storage-url", config.Storage.URL, "Storage URL, e.g. s3://my-bucket/my-folder")
		flag.Parse()
	})
	return config
}
