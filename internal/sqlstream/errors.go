package sqlstream

import "errors"

var (
	// ErrInvalidDatabaseName is returned when a database name cannot be used
	// to build a per-database file name.
	ErrInvalidDatabaseName = errors.New("invalid database name")

	// ErrDuplicateDatabase is returned when a stream carries the same
	// database more than once.
	ErrDuplicateDatabase = errors.New("duplicate database in stream")

	// ErrNoMarker is returned when a stream carries no database marker.
	ErrNoMarker = errors.New("no database marker found")

	// ErrUnexpectedContent is returned when a stream carries content before
	// the first database marker.
	ErrUnexpectedContent = errors.New("unexpected content before the first database marker")

	// ErrNonRegularTarget is returned when the dump file of a database in the
	// target directory exists but is not a regular file, for example a
	// symbolic link planted by someone else with write access to the
	// directory.
	ErrNonRegularTarget = errors.New("dump file target is not a regular file")
)
