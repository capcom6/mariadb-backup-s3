package logical

import "errors"

// ErrInvalidMode is returned when a backup method is used in a mode it does
// not support.
var ErrInvalidMode = errors.New("invalid backup mode")
