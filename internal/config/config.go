package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
)

const (
	BackupMethodPhysical = "mariadb-backup"
	BackupMethodLogical  = "mariadb-dump"
)

var (
	ErrInvalidBackupMethod = errors.New("invalid backup method")
)

type MariaDB struct {
	Host          string
	Port          int
	User          string
	Password      string
	BackupOptions string
	BackupBinary  string
	ClientBinary  string
	BackupMethod  string
}

func (m MariaDB) IsLogical() bool {
	return m.BackupMethod == BackupMethodLogical
}

func (m MariaDB) Validate() error {
	if m.BackupMethod != BackupMethodPhysical && m.BackupMethod != BackupMethodLogical {
		return fmt.Errorf(
			"%w: %q: must be %q or %q",
			ErrInvalidBackupMethod, m.BackupMethod, BackupMethodPhysical, BackupMethodLogical,
		)
	}

	if _, err := exec.LookPath(m.BackupBinary); err != nil {
		return fmt.Errorf("binary '%s' not found in PATH: %w", m.BackupBinary, err)
	}

	if m.IsLogical() {
		if _, err := exec.LookPath(m.ClientBinary); err != nil {
			return fmt.Errorf("client binary '%s' not found in PATH: %w", m.ClientBinary, err)
		}
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
		ClientBinary:  "mariadb",
		BackupMethod:  BackupMethodPhysical,
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
