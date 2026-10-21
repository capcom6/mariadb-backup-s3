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
)
