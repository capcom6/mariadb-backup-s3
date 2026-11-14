package backup

import "errors"

var ErrExternalCommandFailed = errors.New("external command failed")
var ErrValidationFailed = errors.New("validation failed")
