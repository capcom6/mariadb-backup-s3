package logging

import (
	"context"
	"maps"
	"sync"
)

type TestLogEntry struct {
	Level   LogLevel
	Message string
	Err     error
	Fields  Fields
}

type TestLogger struct {
	mu      *sync.Mutex
	entries []TestLogEntry
	level   LogLevel
	ctx     LogContext
}

func NewTestLogger() *TestLogger {
	return &TestLogger{
		mu:      &sync.Mutex{},
		entries: make([]TestLogEntry, 0),
		level:   LogLevelDebug,
		ctx: LogContext{
			Component:   "",
			OperationID: "",
			Fields:      make(Fields),
		},
	}
}

func (l *TestLogger) Debug(ctx context.Context, message string, fields ...Fields) {
	l.log(ctx, LogLevelDebug, message, nil, fields...)
}

func (l *TestLogger) Info(ctx context.Context, message string, fields ...Fields) {
	l.log(ctx, LogLevelInfo, message, nil, fields...)
}

func (l *TestLogger) Warn(ctx context.Context, message string, fields ...Fields) {
	l.log(ctx, LogLevelWarn, message, nil, fields...)
}

func (l *TestLogger) Error(ctx context.Context, message string, err error, fields ...Fields) {
	l.log(ctx, LogLevelError, message, err, fields...)
}

func (l *TestLogger) Fatal(ctx context.Context, message string, err error, fields ...Fields) {
	l.log(ctx, LogLevelFatal, message, err, fields...)
}

func (l *TestLogger) WithContext(component string, operationID string, fields ...Fields) Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	mergedFields := make(Fields)
	maps.Copy(mergedFields, l.ctx.Fields)
	for _, f := range fields {
		maps.Copy(mergedFields, f)
	}

	return &TestLogger{
		mu:      l.mu,
		entries: l.entries,
		level:   l.level,
		ctx: LogContext{
			Component:   component,
			OperationID: operationID,
			Fields:      mergedFields,
		},
	}
}

func (l *TestLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *TestLogger) SetFormatter(_ Formatter) {}

func (l *TestLogger) Flush() error { return nil }

func (l *TestLogger) Close() error { return nil }

func (l *TestLogger) Entries() []TestLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	result := make([]TestLogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

func (l *TestLogger) log(ctx context.Context, level LogLevel, message string, err error, fields ...Fields) {
	component := getComponent(ctx)
	operationID := getOperationID(ctx)
	contextFields := getFields(ctx)

	if component == "" {
		component = l.ctx.Component
	}
	if operationID == "" {
		operationID = l.ctx.OperationID
	}

	mergedFields := make(Fields)
	maps.Copy(mergedFields, l.ctx.Fields)
	maps.Copy(mergedFields, contextFields)
	for _, f := range fields {
		maps.Copy(mergedFields, f)
	}

	mergedFields["component"] = component
	mergedFields["operation_id"] = operationID

	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	l.entries = append(l.entries, TestLogEntry{
		Level:   level,
		Message: message,
		Err:     err,
		Fields:  mergedFields,
	})
}
