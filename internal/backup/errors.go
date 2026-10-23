package backup

import "errors"

var (
	ErrExternalCommandFailed = errors.New("external command failed")
	ErrEngineNotImplemented  = errors.New("database engine backup is not implemented")
)
