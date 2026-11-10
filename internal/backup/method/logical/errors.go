package logical

import "errors"

// ErrInvalidMode is returned when a backup method is used in a mode it does
// not support.
var ErrInvalidMode = errors.New("invalid backup mode")

// ErrNoDatabases is returned when a stream backup finds no databases to dump.
var ErrNoDatabases = errors.New("no databases found to stream")
