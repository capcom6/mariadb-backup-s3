package restore

import "errors"

var (
	ErrValidationFailed      = errors.New("validation failed")
	ErrExternalCommandFailed = errors.New("external command failed")
)
