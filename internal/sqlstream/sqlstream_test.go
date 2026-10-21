package sqlstream_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/sqlstream"
)

func TestWriteMarker(t *testing.T) {
	var buf strings.Builder

	require.NoError(t, sqlstream.WriteMarker(&buf, "mydb"))
	assert.Equal(t, "-- mariadb-backup-s3: database=mydb\n", buf.String())
}

func TestWriteMarker_InvalidDatabaseName(t *testing.T) {
	var buf strings.Builder

	err := sqlstream.WriteMarker(&buf, "../escape")
	require.Error(t, err)
	require.ErrorIs(t, err, sqlstream.ErrInvalidDatabaseName)
	assert.Empty(t, buf.String())
}

func TestParseMarker(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
		ok       bool
	}{
		{name: "marker", line: "-- mariadb-backup-s3: database=mydb\n", expected: "mydb", ok: true},
		{name: "marker without newline", line: "-- mariadb-backup-s3: database=mydb", expected: "mydb", ok: true},
		{name: "marker with crlf", line: "-- mariadb-backup-s3: database=mydb\r\n", expected: "mydb", ok: true},
		{name: "not a marker", line: "SELECT 1;\n", expected: "", ok: false},
		{name: "empty line", line: "\n", expected: "", ok: false},
		{name: "marker prefix only", line: "-- mariadb-backup-s3: database=\n", expected: "", ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ok := sqlstream.ParseMarker(tt.line)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.expected, db)
		})
	}
}

func TestValidateDatabaseName(t *testing.T) {
	tests := []struct {
		name    string
		db      string
		wantErr bool
	}{
		{name: "simple", db: "mydb", wantErr: false},
		{name: "with dash and underscore", db: "my-db_1", wantErr: false},
		{name: "empty", db: "", wantErr: true},
		{name: "dot", db: ".", wantErr: true},
		{name: "dot dot", db: "..", wantErr: true},
		{name: "path separator", db: "db/child", wantErr: true},
		{name: "windows path separator", db: `db\child`, wantErr: true},
		{name: "traversal", db: "../escape", wantErr: true},
		{name: "newline", db: "db\nother", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sqlstream.ValidateDatabaseName(tt.db)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, sqlstream.ErrInvalidDatabaseName)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSplitToDir(t *testing.T) {
	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n" +
		"CREATE DATABASE `mydb`;\n" +
		"USE `mydb`;\n" +
		"-- mariadb-backup-s3: database=otherdb\n" +
		"CREATE DATABASE `otherdb`;\n" +
		"USE `otherdb`;\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "CREATE DATABASE `mydb`;\nUSE `mydb`;\n")
	assertFileContent(t, filepath.Join(dir, "otherdb.sql"), "CREATE DATABASE `otherdb`;\nUSE `otherdb`;\n")
}

func TestSplitToDir_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\nSELECT 1;"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "SELECT 1;")
}

func TestSplitToDir_EmptyDatabase(t *testing.T) {
	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "")
}

func TestSplitToDir_PreservesEmptyLines(t *testing.T) {
	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n" +
		"CREATE DATABASE `mydb`;\n" +
		"\n" +
		"USE `mydb`;\n" +
		"\n" +
		"INSERT INTO t VALUES (1);\n" +
		"\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"),
		"CREATE DATABASE `mydb`;\n\nUSE `mydb`;\n\nINSERT INTO t VALUES (1);\n\n")
}

func TestSplitToDir_NoMarker(t *testing.T) {
	dir := t.TempDir()

	err := sqlstream.SplitToDir(strings.NewReader("SELECT 1;\n"), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrUnexpectedContent)
}

func TestSplitToDir_EmptyStream(t *testing.T) {
	dir := t.TempDir()

	err := sqlstream.SplitToDir(strings.NewReader(""), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrNoMarker)
}

func TestSplitToDir_InvalidDatabaseName(t *testing.T) {
	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=../escape\nSELECT 1;\n"

	err := sqlstream.SplitToDir(strings.NewReader(stream), dir)
	require.Error(t, err)
	require.ErrorIs(t, err, sqlstream.ErrInvalidDatabaseName)

	_, statErr := os.Stat(filepath.Join(dir, "..", "escape.sql"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSplitToDir_DuplicateDatabase(t *testing.T) {
	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\nSELECT 1;\n" +
		"-- mariadb-backup-s3: database=mydb\nSELECT 2;\n"

	err := sqlstream.SplitToDir(strings.NewReader(stream), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrDuplicateDatabase)
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(content))
}
