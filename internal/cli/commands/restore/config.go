package restore

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

type MariaDBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

type StorageConfig struct {
	URL string
}

func (s StorageConfig) GetURL() (*url.URL, error) {
	if s.URL == "" {
		return nil, fmt.Errorf("storage URL is empty")
	}

	return url.Parse(s.URL)
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

type RestoreConfig struct {
	TargetDir string
}

type Config struct {
	MariaDB    MariaDBConfig
	Storage    StorageConfig
	Encryption EncryptionConfig
	Restore    RestoreConfig
}

func DefaultConfig() Config {
	return Config{
		MariaDB: MariaDBConfig{
			Host: "localhost",
			Port: 3306,
			User: "root",
		},
		Storage: StorageConfig{
			URL: "",
		},
		Encryption: EncryptionConfig{
			EncryptionKey: "",
		},
		Restore: RestoreConfig{
			TargetDir: "",
		},
	}
}
