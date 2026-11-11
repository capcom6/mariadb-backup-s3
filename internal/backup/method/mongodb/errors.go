package mongodb

import "errors"

// ErrInvalidMode is returned when a backup method is used in a mode it does
// not support.
var ErrInvalidMode = errors.New("invalid backup mode")

// ErrOptionConflictsWithStream is returned when a backup option would
// conflict with the managed streaming output: --archive would redirect the
// dump away from stdout, and --gzip would nest a compression layer the
// pipeline already produces, yielding an artifact restore cannot read.
var ErrOptionConflictsWithStream = errors.New("backup option is not supported in streaming mode")
