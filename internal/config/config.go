package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
)

const (
	EngineMariaDB = "mariadb"
	EngineMongoDB = "mongodb"

	BackupMethodPhysical = "mariadb-backup"
	BackupMethodLogical  = "mariadb-dump"
	BackupMethodMongoDB  = "mongodump"

	// defaultMongoAuthSource is the MongoDB authentication database applied
	// when engine is mongodb and no auth source was provided (CLI parity).
	defaultMongoAuthSource = "admin"
)

var (
	ErrInvalidEngine            = errors.New("invalid database engine")
	ErrInvalidBackupMethod      = errors.New("invalid backup method")
	ErrIncompatibleBackupMethod = errors.New("backup method is incompatible with database engine")
	ErrBackupBinaryRequired     = errors.New("backup binary is required")
)

type Database struct {
	Engine string

	Host     string
	Port     int
	User     string
	Password string

	BackupOptions string

	BackupBinary string
	ClientBinary string

	BackupMethod string

	MongoDB MongoDB
}

type MongoDB struct {
	URI                   string
	AuthSource            string
	TLS                   bool
	TLSCAFile             string
	TLSCertificateKeyFile string
}

func (d Database) IsLogical() bool {
	return d.BackupMethod == BackupMethodLogical
}

// Normalize returns a copy with engine-aware defaults applied to empty fields.
func (d Database) Normalize() Database {
	d.Engine = normalizeEngine(d.Engine)

	defaults := DefaultDatabase(d.Engine)

	if d.Host == "" {
		d.Host = defaults.Host
	}
	if d.Port == 0 {
		d.Port = defaults.Port
	}
	if d.User == "" {
		d.User = defaults.User
	}
	if d.BackupMethod == "" {
		d.BackupMethod = defaults.BackupMethod
	}
	if d.BackupBinary == "" {
		d.BackupBinary = DefaultBackupBinary(d.Engine, d.BackupMethod)
	}
	if d.ClientBinary == "" {
		d.ClientBinary = defaults.ClientBinary
	}
	if d.Engine == EngineMongoDB && d.MongoDB.AuthSource == "" {
		d.MongoDB.AuthSource = defaultMongoAuthSource
	}

	return d
}

func (d Database) Validate() error {
	normalized := d.Normalize()

	if err := normalized.validateEngine(); err != nil {
		return err
	}
	if err := normalized.validateBackupMethod(); err != nil {
		return err
	}

	return normalized.validateBinaries()
}

func (d Database) validateEngine() error {
	switch d.Engine {
	case EngineMariaDB, EngineMongoDB:
		return nil
	default:
		return fmt.Errorf(
			"%w: %q: must be %q or %q",
			ErrInvalidEngine, d.Engine, EngineMariaDB, EngineMongoDB,
		)
	}
}

func (d Database) validateBackupMethod() error {
	switch d.BackupMethod {
	case BackupMethodPhysical, BackupMethodLogical:
		if d.Engine != EngineMariaDB {
			return fmt.Errorf(
				"%w: %q for engine %q",
				ErrIncompatibleBackupMethod, d.BackupMethod, d.Engine,
			)
		}
	case BackupMethodMongoDB:
		if d.Engine != EngineMongoDB {
			return fmt.Errorf(
				"%w: %q for engine %q",
				ErrIncompatibleBackupMethod, d.BackupMethod, d.Engine,
			)
		}
	default:
		return fmt.Errorf(
			"%w: %q: must be %q, %q or %q",
			ErrInvalidBackupMethod,
			d.BackupMethod,
			BackupMethodPhysical,
			BackupMethodLogical,
			BackupMethodMongoDB,
		)
	}

	return nil
}

func (d Database) validateBinaries() error {
	if d.BackupBinary == "" {
		return fmt.Errorf("%w for engine %q", ErrBackupBinaryRequired, d.Engine)
	}

	if _, err := exec.LookPath(d.BackupBinary); err != nil {
		return fmt.Errorf("binary '%s' not found in PATH: %w", d.BackupBinary, err)
	}

	if d.Engine == EngineMongoDB {
		return nil
	}

	if d.IsLogical() {
		if _, err := exec.LookPath(d.ClientBinary); err != nil {
			return fmt.Errorf("client binary '%s' not found in PATH: %w", d.ClientBinary, err)
		}
	}

	return nil
}

// DefaultBackupBinary returns the binary for an engine and backup method pair.
func DefaultBackupBinary(engine, backupMethod string) string {
	switch backupMethod {
	case BackupMethodLogical:
		return "mariadb-dump"
	case BackupMethodMongoDB:
		return "mongodump"
	case BackupMethodPhysical:
		return "mariadb-backup"
	default:
		return defaultBackupBinaryForEngine(engine)
	}
}

func DefaultDatabase(engine string) Database {
	engine = normalizeEngine(engine)

	db := Database{
		Engine:        engine,
		Host:          "",
		Port:          0,
		User:          "",
		Password:      "",
		BackupOptions: "",
		BackupBinary:  "",
		ClientBinary:  "",
		BackupMethod:  "",
		MongoDB: MongoDB{
			URI:                   "",
			AuthSource:            "",
			TLS:                   false,
			TLSCAFile:             "",
			TLSCertificateKeyFile: "",
		},
	}

	switch engine {
	case EngineMariaDB:
		db.Host = "localhost"
		db.Port = 3306
		db.User = "root"
		db.BackupMethod = BackupMethodPhysical
		db.ClientBinary = "mariadb"
	case EngineMongoDB:
		db.Host = "localhost"
		db.Port = 27017
		db.BackupMethod = BackupMethodMongoDB
	}

	db.BackupBinary = DefaultBackupBinary(engine, db.BackupMethod)

	return db
}

func normalizeEngine(engine string) string {
	if engine == "" {
		return EngineMariaDB
	}

	return engine
}

func defaultBackupBinaryForEngine(engine string) string {
	if engine == EngineMongoDB {
		return "mongodump"
	}

	return "mariadb-backup"
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
