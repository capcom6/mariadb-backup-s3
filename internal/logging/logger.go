package logging

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"sync"
	"time"
)

// loggerImpl is the main implementation of the Logger interface.
type loggerImpl struct {
	// mu protects concurrent access to logger state
	mu sync.RWMutex
	// level is the minimum log level to output
	level LogLevel
	// formatter is the log formatter to use
	formatter Formatter
	// writer is the output writer
	writer io.Writer
	// context holds default context information
	ctx LogContext
}

// New creates a new logger instance.
func New(config Config) (Logger, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Get output writer
	writer := config.Output
	if writer == nil {
		writer = os.Stdout
	}

	// Create formatter based on format
	var formatter Formatter
	switch config.Format {
	case FormatJSON:
		formatter = NewJSONFormatter(config.TimeFormat)
	case FormatHuman:
		formatter = NewHumanFormatter(config.EnableColors, config.TimeFormat)
	default:
		return nil, fmt.Errorf("%w: unsupported format: %s", ErrValidationFailed, config.Format)
	}

	return &loggerImpl{
		level:     config.Level,
		formatter: formatter,
		writer:    writer,
		ctx: LogContext{
			Fields:      make(Fields),
			Component:   "",
			OperationID: "",
		},
		mu: sync.RWMutex{},
	}, nil
}

// NewDefault creates a new logger with default configuration.
func NewDefault() Logger {
	config := DefaultConfig()
	logger, err := New(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create default logger: %v", err))
	}
	return logger
}

// Debug logs a message at debug level.
func (l *loggerImpl) Debug(ctx context.Context, message string, fields ...Fields) {
	l.log(ctx, LogLevelDebug, message, nil, fields...)
}

// Info logs a message at info level.
func (l *loggerImpl) Info(ctx context.Context, message string, fields ...Fields) {
	l.log(ctx, LogLevelInfo, message, nil, fields...)
}

// Warn logs a message at warn level.
func (l *loggerImpl) Warn(ctx context.Context, message string, fields ...Fields) {
	l.log(ctx, LogLevelWarn, message, nil, fields...)
}

// Error logs a message at error level.
func (l *loggerImpl) Error(ctx context.Context, message string, err error, fields ...Fields) {
	l.log(ctx, LogLevelError, message, err, fields...)
}

// Fatal logs a message at fatal level and terminates the application.
func (l *loggerImpl) Fatal(ctx context.Context, message string, err error, fields ...Fields) {
	l.log(ctx, LogLevelFatal, message, err, fields...)
	// Ensure buffered writers flush before exit
	_ = l.Flush()
	// Terminate the application
	os.Exit(1)
}

// WithContext creates a new logger with the specified context.
func (l *loggerImpl) WithContext(component string, operationID string, fields ...Fields) Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Merge fields
	mergedFields := make(Fields)
	maps.Copy(mergedFields, l.ctx.Fields)

	for _, fieldMap := range fields {
		maps.Copy(mergedFields, fieldMap)
	}

	return &loggerImpl{
		level:     l.level,
		formatter: l.formatter,
		writer:    l.writer,
		ctx: LogContext{
			Component:   component,
			OperationID: operationID,
			Fields:      mergedFields,
		},
		mu: sync.RWMutex{},
	}
}

// SetLevel sets the minimum log level.
func (l *loggerImpl) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetFormatter sets the log formatter.
func (l *loggerImpl) SetFormatter(formatter Formatter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.formatter = formatter
}

// Flush ensures all log entries are written.
func (l *loggerImpl) Flush() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if flusher, ok := l.writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return fmt.Errorf("failed to flush log writer: %w", err)
		}
	}
	return nil
}

// Close closes the logger.
func (l *loggerImpl) Close() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if closer, ok := l.writer.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("failed to close log writer: %w", err)
		}
	}
	return nil
}

// log is the internal method that handles the actual logging.
func (l *loggerImpl) log(ctx context.Context, level LogLevel, message string, err error, fields ...Fields) {
	// Check if we should log this level
	if !l.shouldLog(level) {
		return
	}

	// Get context information
	component := getComponent(ctx)
	operationID := getOperationID(ctx)
	contextFields := getFields(ctx)

	// Use logger context as fallback
	if component == "" {
		component = l.ctx.Component
	}
	if operationID == "" {
		operationID = l.ctx.OperationID
	}

	// Merge all fields
	mergedFields := make(Fields)
	maps.Copy(mergedFields, l.ctx.Fields)
	maps.Copy(mergedFields, contextFields)
	for _, fieldMap := range fields {
		maps.Copy(mergedFields, fieldMap)
	}

	// Create log entry
	entry := &LogEntry{
		Timestamp:   time.Now(),
		Level:       level,
		Message:     message,
		Component:   component,
		OperationID: operationID,
		Fields:      mergedFields,
		Error:       err,
	}

	// Format and write the log entry
	l.mu.RLock()
	formatter := l.formatter
	writer := l.writer
	l.mu.RUnlock()

	if fmtErr := formatter.Format(entry, writer); fmtErr != nil {
		// If formatting fails, write to stderr as fallback
		fmt.Fprintf(os.Stderr, "LOGGING ERROR: failed to format log entry: %v\n", fmtErr)
	}
}

// shouldLog checks if a log level should be logged based on current level.
func (l *loggerImpl) shouldLog(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}
