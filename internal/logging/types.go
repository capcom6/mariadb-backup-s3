package logging

import (
	"context"
	"time"
)

// LogLevel represents the severity level of log entries.
type LogLevel int

const (
	// LogLevelDebug is the lowest level, used for detailed debugging information.
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is used for general information messages.
	LogLevelInfo
	// LogLevelWarn is used for warning messages.
	LogLevelWarn
	// LogLevelError is used for error messages.
	LogLevelError
	// LogLevelFatal is used for fatal error messages that terminate the application.
	LogLevelFatal
)

// MarshalText makes LogLevel encode as its string (e.g., "INFO") in JSON and text encoders.
func (l LogLevel) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a structured log entry with all necessary information.
type LogEntry struct {
	Timestamp   time.Time      // Timestamp when the log entry was created
	Level       LogLevel       // Level of the log entry
	Message     string         // Message is the main log message
	Component   string         // Component is the component or module that generated the log entry
	OperationID string         // OperationID is an optional identifier for grouping related log entries
	Fields      map[string]any // Fields contains additional structured data
	Error       error          // Error contains error information if the log entry is error-related
}

// LogContext holds contextual information for logging operations.
type LogContext struct {
	// Component is the component or module name
	Component string
	// OperationID is an identifier for the current operation
	OperationID string
	// Fields contains additional context fields
	Fields map[string]any
}

// Logger defines the interface for logging operations.
type Logger interface {
	// Debug logs a message at debug level
	Debug(ctx context.Context, message string, fields ...Fields)
	// Info logs a message at info level
	Info(ctx context.Context, message string, fields ...Fields)
	// Warn logs a message at warn level
	Warn(ctx context.Context, message string, fields ...Fields)
	// Error logs a message at error level
	Error(ctx context.Context, message string, err error, fields ...Fields)
	// Fatal logs a message at fatal level and terminates the application
	Fatal(ctx context.Context, message string, err error, fields ...Fields)

	// WithContext creates a new logger with the specified context
	WithContext(component string, operationID string, fields ...Fields) Logger
	// SetLevel sets the minimum log level
	SetLevel(level LogLevel)
	// SetFormatter sets the log formatter
	SetFormatter(formatter Formatter)
	// Flush ensures all log entries are written
	Flush() error

	// Close closes the logger
	Close() error
}

// Fields is a type for log fields to avoid confusion with map parameters.
type Fields map[string]any
