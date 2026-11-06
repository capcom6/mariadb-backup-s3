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
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, sqlstream.WriteMarker(&buf, "mydb"))
	assert.Equal(t, "-- mariadb-backup-s3: database=mydb\n", buf.String())
}

func TestWriteMarker_InvalidDatabaseName(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := sqlstream.WriteMarker(&buf, "../escape")
	require.Error(t, err)
	require.ErrorIs(t, err, sqlstream.ErrInvalidDatabaseName)
	assert.Empty(t, buf.String())
}

func TestParseMarker(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			db, ok := sqlstream.ParseMarker(tt.line)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.expected, db)
		})
	}
}

func TestValidateDatabaseName(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\nSELECT 1;"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "SELECT 1;")
}

func TestSplitToDir_EmptyDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "")
}

func TestSplitToDir_PreservesEmptyLines(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	dir := t.TempDir()

	err := sqlstream.SplitToDir(strings.NewReader("SELECT 1;\n"), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrUnexpectedContent)
}

func TestSplitToDir_EmptyStream(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	err := sqlstream.SplitToDir(strings.NewReader(""), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrNoMarker)
}

func TestSplitToDir_InvalidDatabaseName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=../escape\nSELECT 1;\n"

	err := sqlstream.SplitToDir(strings.NewReader(stream), dir)
	require.Error(t, err)
	require.ErrorIs(t, err, sqlstream.ErrInvalidDatabaseName)

	_, statErr := os.Stat(filepath.Join(dir, "..", "escape.sql"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSplitToDir_DuplicateDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\nSELECT 1;\n" +
		"-- mariadb-backup-s3: database=mydb\nSELECT 2;\n"

	err := sqlstream.SplitToDir(strings.NewReader(stream), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrDuplicateDatabase)
}

func TestSplitToDir_LongLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Longer than bufio's 4 KiB buffer, so the reader sees ErrBufferFull and
	// the line arrives in several chunks.
	longLine := strings.Repeat("x", 10_000)

	stream := "-- mariadb-backup-s3: database=mydb\n" +
		longLine + "\n" +
		"SELECT 1;\n" +
		"-- mariadb-backup-s3: database=otherdb\n" +
		longLine + "\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), longLine+"\nSELECT 1;\n")
	assertFileContent(t, filepath.Join(dir, "otherdb.sql"), longLine+"\n")
}

func TestSplitToDir_LongLineWithoutMarker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	err := sqlstream.SplitToDir(strings.NewReader(strings.Repeat("x", 10_000)+"\n"), dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlstream.ErrUnexpectedContent)
}

func TestEscaper_EscapesMarkerLikeLine(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	e := sqlstream.NewEscaper(&buf)

	_, err := e.Write([]byte("-- mariadb-backup-s3: database=evil\nSELECT 1;\n"))
	require.NoError(t, err)
	require.NoError(t, e.Flush())

	assert.Equal(t, "\x00-- mariadb-backup-s3: database=evil\nSELECT 1;\n", buf.String())
}

func TestEscaper_EscapesMarkerAcrossWrites(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	e := sqlstream.NewEscaper(&buf)

	for _, part := range []string{"-- mariadb-backup", "-s3: data", "base=mydb\n"} {
		_, err := e.Write([]byte(part))
		require.NoError(t, err)
	}
	require.NoError(t, e.Flush())

	assert.Equal(t, "\x00-- mariadb-backup-s3: database=mydb\n", buf.String())
}

func TestEscaper_EscapesEscapeByteLine(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	e := sqlstream.NewEscaper(&buf)

	_, err := e.Write([]byte("\x00weird\n"))
	require.NoError(t, err)
	require.NoError(t, e.Flush())

	assert.Equal(t, "\x00\x00weird\n", buf.String())
}

func TestEscaper_LeavesContentUntouched(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	e := sqlstream.NewEscaper(&buf)

	_, err := e.Write([]byte("-- MySQL dump\nSELECT '-- mariadb-backup-s3: database=x';\n"))
	require.NoError(t, err)
	require.NoError(t, e.Flush())

	assert.Equal(t, "-- MySQL dump\nSELECT '-- mariadb-backup-s3: database=x';\n", buf.String())
}

func TestEscaper_FlushEmitsBufferedPrefix(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	e := sqlstream.NewEscaper(&buf)

	_, err := e.Write([]byte("-- mari"))
	require.NoError(t, err)
	require.NoError(t, e.Flush())

	assert.Equal(t, "-- mari", buf.String())
}

func TestSplitToDir_EscapedMarkerLikeContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n" +
		"\x00-- mariadb-backup-s3: database=evil\nSELECT 1;\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"),
		"-- mariadb-backup-s3: database=evil\nSELECT 1;\n")
	_, err := os.Stat(filepath.Join(dir, "evil.sql"))
	assert.True(t, os.IsNotExist(err))
}

func TestSplitToDir_EscapedEscapeByteContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n" + "\x00\x00weird\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "\x00weird\n")
}

func TestSplitToDir_RawEscapeByteLinePreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stream := "-- mariadb-backup-s3: database=mydb\n" + "\x00raw\n"

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(stream), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), "\x00raw\n")
}

func TestEscaper_SplitRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	content := "-- comment\n" +
		"-- mariadb-backup-s3: database=evil\n" +
		"\x00raw\n" +
		"SELECT '-- mariadb-backup-s3: database=inline';\n" +
		"INSERT INTO t VALUES (1);\n"

	var buf strings.Builder
	require.NoError(t, sqlstream.WriteMarker(&buf, "mydb"))
	e := sqlstream.NewEscaper(&buf)
	_, err := e.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, e.Flush())

	require.NoError(t, sqlstream.SplitToDir(strings.NewReader(buf.String()), dir))

	assertFileContent(t, filepath.Join(dir, "mydb.sql"), content)
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(content))
}
