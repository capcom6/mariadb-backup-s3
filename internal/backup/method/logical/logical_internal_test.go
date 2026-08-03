package logical

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/backup/method"
)

func TestConnectionArgs_AppliesSharedOptionsToDiscovery(t *testing.T) {
	m := New(method.Config{
		Host:          "db.example.com",
		Port:          3307,
		User:          "backup",
		BackupOptions: "--single-transaction --socket=/var/run/mysqld/mysqld.sock --ssl",
	})

	connArgs, err := m.connectionArgs()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"--host=db.example.com",
		"--port=3307",
		"--user=backup",
		"--socket=/var/run/mysqld/mysqld.sock",
		"--ssl",
	}, connArgs)

	dumpArgs, err := m.dumpOnlyArgs()
	require.NoError(t, err)
	assert.Equal(t, []string{"--single-transaction"}, dumpArgs)
}

func TestConnectionArgs_EmptyOptions(t *testing.T) {
	m := New(method.Config{
		Host: "localhost",
		User: "root",
	})

	connArgs, err := m.connectionArgs()
	require.NoError(t, err)
	assert.Equal(t, []string{"--host=localhost", "--user=root"}, connArgs)

	dumpArgs, err := m.dumpOnlyArgs()
	require.NoError(t, err)
	assert.Empty(t, dumpArgs)
}
