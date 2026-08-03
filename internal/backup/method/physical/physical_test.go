package physical_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/physical"
)

func TestNew(t *testing.T) {
	cfg := method.Config{
		Host:         "localhost",
		Port:         3306,
		User:         "root",
		Password:     "secret",
		BackupBinary: "mariadb-backup",
	}

	m := physical.New(cfg)
	assert.NotNil(t, m)
}

func TestMethod_MethodName(t *testing.T) {
	cfg := method.Config{
		BackupBinary: "mariadb-backup",
	}

	m := physical.New(cfg)
	assert.Equal(t, physical.MethodName, m.MethodName())
}

func TestMethod_ImplementsInterface(_ *testing.T) {
	cfg := method.Config{
		BackupBinary: "mariadb-backup",
	}

	m := physical.New(cfg)
	var _ method.Method = m
}
