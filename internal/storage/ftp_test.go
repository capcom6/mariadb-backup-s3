package storage_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

type testFTPError struct {
	code    int
	message string
}

func (e testFTPError) Error() string {
	return e.message
}

func (e testFTPError) Code() int {
	return e.code
}

func (e testFTPError) Message() string {
	return e.message
}

func (e testFTPError) Temporary() bool {
	return false
}

func TestValidateFilename_Valid(t *testing.T) {
	s := storage.NewTestFTPStorage("/base")
	tests := []struct {
		filename string
		expected string
	}{
		{"backup.tar.gz", "backup.tar.gz"},
		{"path/to/file.txt", "path/to/file.txt"},
		{"normal_file.dat", "normal_file.dat"},
		{"with.dots.everywhere", "with.dots.everywhere"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := s.ValidateFilename(tt.filename)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestValidateFilename_Empty(t *testing.T) {
	s := storage.NewTestFTPStorage("/base")
	_, err := s.ValidateFilename("")
	assert.ErrorIs(t, err, storage.ErrInvalidArgument)
}

func TestValidateFilename_Absolute(t *testing.T) {
	s := storage.NewTestFTPStorage("/base")
	_, err := s.ValidateFilename("/etc/passwd")
	assert.ErrorIs(t, err, storage.ErrInvalidArgument)
}

func TestValidateFilename_NormalizesTraversal(t *testing.T) {
	s := storage.NewTestFTPStorage("/base")
	tests := []struct {
		name     string
		filename string
		expected string
	}{
		{"parent dir", "../outside.txt", "outside.txt"},
		{"deep traversal", "../../../etc/passwd", "etc/passwd"},
		{"encoded traversal", "foo/../../bar.txt", "bar.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ValidateFilename(tt.filename)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got,
				"filename %q should be normalized", tt.filename)
		})
	}
}

func TestValidateFilename_DotDot(t *testing.T) {
	s := storage.NewTestFTPStorage("/base")
	tests := []struct {
		name     string
		filename string
	}{
		{"dot dot only", ".."},
		{"dot dot slash", "../"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ValidateFilename(tt.filename)
			require.NoError(t, err)
			assert.Empty(t, got, "filename %q should normalize to empty path", tt.filename)
		})
	}
}

func TestValidateFilename_TrailingSlash(t *testing.T) {
	s := storage.NewTestFTPStorage("/base")
	got, err := s.ValidateFilename("dir/")
	require.NoError(t, err)
	assert.Equal(t, "dir", got, "trailing slash should be normalized away")
}

func TestIsFTPNotFoundError_Code550(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"no such file", "No such file or directory"},
		{"file not found", "File not found"},
		{"not found", "Not found"},
		{"does not exist", "The file does not exist"},
		{"cannot find", "Cannot find the file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testFTPError{code: 550, message: tt.message}
			assert.True(t, storage.IsFTPNotFoundError(err),
				"expected true for code 550 with message %q", tt.message)
		})
	}
}

func TestIsFTPNotFoundError_Code550_WrongMessage(t *testing.T) {
	err := testFTPError{code: 550, message: "Permission denied"}
	assert.False(t, storage.IsFTPNotFoundError(err),
		"code 550 without 'not found' message should return false")
}

func TestIsFTPNotFoundError_OtherCode(t *testing.T) {
	err := testFTPError{code: 530, message: "Not found"}
	assert.False(t, storage.IsFTPNotFoundError(err),
		"code 530 should return false even with 'not found' message")
}

func TestIsFTPNotFoundError_NonFTPError(t *testing.T) {
	err := errors.New("some generic error")
	assert.False(t, storage.IsFTPNotFoundError(err))
}

func TestIsFTPNotFoundError_NilError(t *testing.T) {
	assert.False(t, storage.IsFTPNotFoundError(nil))
}
