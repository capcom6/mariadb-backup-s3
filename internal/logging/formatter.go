package logging

import (
	"io"
)

// Formatter defines the interface for formatting log entries.
type Formatter interface {
	// Format formats a log entry for output
	Format(entry *LogEntry, writer io.Writer) error
	// Name returns the name of the formatter
	Name() string
}
