package restore

import "errors"

var (
	ErrBackupNotFound = errors.New("backup not found")
	ErrInvalidParams  = errors.New("invalid parameters")
)
