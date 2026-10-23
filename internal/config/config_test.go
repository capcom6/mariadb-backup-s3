package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

func TestDefaultDatabase_MariaDB(t *testing.T) {
	cfg := config.DefaultDatabase(config.EngineMariaDB)
	assert.Equal(t, config.EngineMariaDB, cfg.Engine)
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 3306, cfg.Port)
	assert.Equal(t, "root", cfg.User)
	assert.Equal(t, "mariadb-backup", cfg.BackupBinary)
	assert.Equal(t, "mariadb", cfg.ClientBinary)
	assert.Empty(t, cfg.RestoreBinary)
	assert.Equal(t, config.BackupMethodPhysical, cfg.BackupMethod)
	assert.False(t, cfg.IsLogical())
}

func TestDefaultDatabase_MongoDB(t *testing.T) {
	cfg := config.DefaultDatabase(config.EngineMongoDB)
	assert.Equal(t, config.EngineMongoDB, cfg.Engine)
	assert.Equal(t, 27017, cfg.Port)
	assert.Equal(t, "mongodump", cfg.BackupBinary)
	assert.Empty(t, cfg.ClientBinary)
	assert.Equal(t, "mongorestore", cfg.RestoreBinary)
	assert.Equal(t, config.BackupMethodMongoDB, cfg.BackupMethod)
	assert.False(t, cfg.IsLogical())
}

func TestDefaultDatabase_EmptyEngineDefaultsToMariaDB(t *testing.T) {
	cfg := config.DefaultDatabase("")
	assert.Equal(t, config.EngineMariaDB, cfg.Engine)
	assert.Equal(t, 3306, cfg.Port)
	assert.Equal(t, config.BackupMethodPhysical, cfg.BackupMethod)
}

func TestDatabase_Normalize_EngineAwareDefaults(t *testing.T) {
	tests := []struct {
		name        string
		engine      string
		wantPort    int
		wantMethod  string
		wantBackup  string
		wantClient  string
		wantRestore string
	}{
		{
			name:        "mariaDB default",
			engine:      "",
			wantPort:    3306,
			wantMethod:  config.BackupMethodPhysical,
			wantBackup:  "mariadb-backup",
			wantClient:  "mariadb",
			wantRestore: "",
		},
		{
			name:        "mongoDB default",
			engine:      config.EngineMongoDB,
			wantPort:    27017,
			wantMethod:  config.BackupMethodMongoDB,
			wantBackup:  "mongodump",
			wantClient:  "",
			wantRestore: "mongorestore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Database{Engine: tt.engine}.Normalize()
			assert.Equal(t, tt.wantPort, cfg.Port)
			assert.Equal(t, tt.wantMethod, cfg.BackupMethod)
			assert.Equal(t, tt.wantBackup, cfg.BackupBinary)
			assert.Equal(t, tt.wantClient, cfg.ClientBinary)
			assert.Equal(t, tt.wantRestore, cfg.RestoreBinary)
		})
	}
}

func TestDatabase_Normalize_PreservesExplicitValues(t *testing.T) {
	cfg := config.Database{
		Engine:        config.EngineMongoDB,
		Host:          "mongo.internal",
		Port:          1234,
		User:          "backup",
		BackupMethod:  config.BackupMethodMongoDB,
		BackupBinary:  "/opt/mongodump",
		ClientBinary:  "/opt/mongo",
		RestoreBinary: "/opt/mongorestore",
	}.Normalize()

	assert.Equal(t, "mongo.internal", cfg.Host)
	assert.Equal(t, 1234, cfg.Port)
	assert.Equal(t, "backup", cfg.User)
	assert.Equal(t, "/opt/mongodump", cfg.BackupBinary)
	assert.Equal(t, "/opt/mongo", cfg.ClientBinary)
	assert.Equal(t, "/opt/mongorestore", cfg.RestoreBinary)
}

// TestDatabase_Normalize_MongoDBAuthSourceDefault mirrors the CLI parse path
// (DefaultDatabase -> flag-derived MongoDB struct overwrite -> Normalize) to
// prove the AuthSource default matches the CLI condition: engine mongodb with
// an unset value becomes "admin", explicit values and non-mongodb engines are
// left untouched.
func TestDatabase_Normalize_MongoDBAuthSourceDefault(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		in     string
		want   string
	}{
		{name: "mongodb unset defaults to admin", engine: config.EngineMongoDB, in: "", want: "admin"},
		{name: "mongodb explicit preserved", engine: config.EngineMongoDB, in: "authdb", want: "authdb"},
		{name: "mariadb unset stays empty", engine: config.EngineMariaDB, in: "", want: ""},
		{name: "empty engine stays empty", engine: "", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := config.DefaultDatabase(tt.engine)
			db.MongoDB = config.MongoDB{AuthSource: tt.in}

			cfg := db.Normalize()

			assert.Equal(t, tt.want, cfg.MongoDB.AuthSource)
		})
	}
}

func TestDatabase_Validate_BinaryFound(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.Database{
		Engine:       config.EngineMariaDB,
		BackupBinary: exe,
		BackupMethod: config.BackupMethodPhysical,
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestDatabase_Validate_BinaryNotFound(t *testing.T) {
	cfg := config.Database{
		Engine:       config.EngineMariaDB,
		BackupBinary: "nonexistent-binary-12345",
		BackupMethod: config.BackupMethodPhysical,
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestDatabase_Validate_InvalidMethod(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.Database{
		Engine:       config.EngineMariaDB,
		BackupBinary: exe,
		BackupMethod: "invalid-method",
	}
	err = cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrInvalidBackupMethod)
}

func TestDatabase_Validate_InvalidEngine(t *testing.T) {
	cfg := config.Database{
		Engine:       "postgres",
		BackupBinary: "nonexistent-binary-12345",
		BackupMethod: config.BackupMethodPhysical,
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrInvalidEngine)
}

func TestDatabase_Validate_MethodEngineMismatch(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		method string
	}{
		{
			name:   "mariadb with mongodump",
			engine: config.EngineMariaDB,
			method: config.BackupMethodMongoDB,
		},
		{
			name:   "mongodb with mariadb-backup",
			engine: config.EngineMongoDB,
			method: config.BackupMethodPhysical,
		},
		{
			name:   "mongodb with mariadb-dump",
			engine: config.EngineMongoDB,
			method: config.BackupMethodLogical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Database{
				Engine:       tt.engine,
				BackupMethod: tt.method,
			}
			err := cfg.Validate()
			require.Error(t, err)
			assert.ErrorIs(t, err, config.ErrIncompatibleBackupMethod)
		})
	}
}

func TestDatabase_Validate_MongoDBDefaults(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.Database{Engine: config.EngineMongoDB, BackupBinary: exe}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestDatabase_Validate_MongoDBBackupBinaryNotFound(t *testing.T) {
	cfg := config.Database{
		Engine:       config.EngineMongoDB,
		BackupBinary: "nonexistent-binary-12345",
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in PATH")
}

func TestDatabase_Validate_MongoDBRestoreBinaryNotRequired(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.Database{
		Engine:        config.EngineMongoDB,
		BackupBinary:  exe,
		RestoreBinary: "nonexistent-restore-binary-12345",
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestDatabase_Validate_LogicalMethod(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.Database{
		Engine:       config.EngineMariaDB,
		BackupBinary: exe,
		ClientBinary: exe,
		BackupMethod: config.BackupMethodLogical,
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestDatabase_Validate_LogicalMethod_ClientBinaryNotFound(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cfg := config.Database{
		Engine:       config.EngineMariaDB,
		BackupBinary: exe,
		ClientBinary: "nonexistent-client-binary-12345",
		BackupMethod: config.BackupMethodLogical,
	}
	err = cfg.Validate()
	assert.Error(t, err)
}

func TestDatabase_IsLogical(t *testing.T) {
	physical := config.Database{BackupMethod: config.BackupMethodPhysical}
	assert.False(t, physical.IsLogical())

	logical := config.Database{BackupMethod: config.BackupMethodLogical}
	assert.True(t, logical.IsLogical())
}

func TestStorage_Validate_ValidURL(t *testing.T) {
	cfg := config.Storage{
		URL: "s3://bucket/path",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestStorage_Validate_InvalidURL(t *testing.T) {
	cfg := config.Storage{
		URL: "://",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestStorage_GetURL_Success(t *testing.T) {
	cfg := config.Storage{
		URL: "s3://mybucket/some/path",
	}
	u, err := cfg.GetURL()
	require.NoError(t, err)
	assert.Equal(t, "s3", u.Scheme)
	assert.Equal(t, "mybucket", u.Host)
	assert.Equal(t, "/some/path", u.Path)
}

func TestStorage_GetURL_Error(t *testing.T) {
	cfg := config.Storage{
		URL: "://",
	}
	_, err := cfg.GetURL()
	assert.Error(t, err)
}

func TestEncryption_Enabled_WithKey(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "dGVzdGtleQ==",
	}
	assert.True(t, cfg.Enabled())
}

func TestEncryption_Enabled_Empty(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "",
	}
	assert.False(t, cfg.Enabled())
}

func TestEncryption_Key_Success(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "dGVzdGtleQ==",
	}
	key, err := cfg.Key()
	require.NoError(t, err)
	assert.Equal(t, []byte("testkey"), key)
}

func TestEncryption_Key_InvalidEncoding(t *testing.T) {
	cfg := config.Encryption{
		EncryptionKey: "not-valid-base64!!!",
	}
	_, err := cfg.Key()
	assert.Error(t, err)
}
