package logical_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
	"github.com/capcom6/mariadb-backup-s3/internal/backup/method/logical"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

func TestNew(t *testing.T) {
	cfg := method.Config{
		Host:         "localhost",
		Port:         3306,
		User:         "root",
		Password:     "secret",
		BackupBinary: "mariadb-dump",
	}

	m := logical.New(cfg)
	assert.NotNil(t, m)
}

func TestMethod_MethodName(t *testing.T) {
	cfg := method.Config{
		BackupBinary: "mariadb-dump",
	}

	m := logical.New(cfg)
	assert.Equal(t, logical.MethodName, m.MethodName())
}

func TestMethod_Prepare_NoOp(t *testing.T) {
	cfg := method.Config{
		BackupBinary: "mariadb-dump",
	}

	m := logical.New(cfg)
	logger := logging.NewTestLogger()
	err := m.Prepare(context.Background(), "/tmp/nonexistent", logger)
	require.NoError(t, err)
}
