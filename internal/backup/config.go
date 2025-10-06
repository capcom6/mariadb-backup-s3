package backup

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

type MariaDBConfig struct {
	Host          string
	Port          int
	User          string
	Password      string
	BackupOptions string
}

type StorageConfig struct {
	URL string
}

func (s StorageConfig) GetURL() (*url.URL, error) {
	return url.Parse(s.URL)
}

type BackupLimitsConfig struct {
	MaxCount int
}

type Backup struct {
	Limits BackupLimitsConfig
}

type EncryptionConfig struct {
	EncryptionKey string
}

func (e EncryptionConfig) Enabled() bool {
	return e.EncryptionKey != ""
}

func (e EncryptionConfig) Key() ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(e.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	return data, nil
}

type Config struct {
	MariaDB    MariaDBConfig
	Storage    StorageConfig
	Backup     Backup
	Encryption EncryptionConfig
}

func DefaultConfig() Config {
	return Config{
		MariaDB: MariaDBConfig{
			Host:          "localhost",
			Port:          3306,
			User:          "root",
			Password:      "",
			BackupOptions: "",
		},
		Storage: StorageConfig{
			URL: "",
		},
		Backup: Backup{
			Limits: BackupLimitsConfig{MaxCount: 0},
		},
		Encryption: EncryptionConfig{
			EncryptionKey: "",
		},
	}
}
