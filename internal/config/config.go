package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os/exec"
)

type MariaDB struct {
	Host          string
	Port          int
	User          string
	Password      string
	BackupOptions string
	BackupBinary  string
}

func (m MariaDB) Validate() error {
	if _, err := exec.LookPath(m.BackupBinary); err != nil {
		return fmt.Errorf("mariabackup binary '%s' not found in PATH: %w", m.BackupBinary, err)
	}

	return nil
}

func DefaultMariaDB() MariaDB {
	const defaultPort = 3306

	return MariaDB{
		Host:          "localhost",
		Port:          defaultPort,
		User:          "root",
		Password:      "",
		BackupOptions: "",
		BackupBinary:  "mariadb-backup",
	}
}

///////////////////////////////////////////////////////////////////////////////

type Storage struct {
	URL string
}

func (s Storage) GetURL() (*url.URL, error) {
	u, err := url.Parse(s.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}

	return u, nil
}

func (s Storage) Validate() error {
	_, err := s.GetURL()

	return err
}

type Encryption struct {
	EncryptionKey string
}

func (e Encryption) Enabled() bool {
	return e.EncryptionKey != ""
}

func (e Encryption) Key() ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(e.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	return data, nil
}
