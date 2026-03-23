package storage

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("object not found")
	ErrLockFailed      = errors.New("lock failed")
)
