package logging

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// HumanFormatter formats log entries in a human-readable format with optional colors.
type HumanFormatter struct {
	// enableColors determines if colors should be used
	enableColors bool
	// timeFormat is the format for timestamps
	timeFormat string
}

// NewHumanFormatter creates a new human formatter.
func NewHumanFormatter(enableColors bool, timeFormat string) *HumanFormatter {
	if timeFormat == "" {
		timeFormat = "2006-01-02 15:04:05.000"
	}
	return &HumanFormatter{
		enableColors: enableColors,
		timeFormat:   timeFormat,
	}
}

// Name returns the formatter name.
func (f *HumanFormatter) Name() string {
	return "human"
}

// Format formats a log entry in human-readable format.
func (f *HumanFormatter) Format(entry *LogEntry, writer io.Writer) error {
	// Format timestamp (default to now if zero)
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	timestamp := ts.Format(f.timeFormat)

	// Format level with color if enabled
	level := f.formatLevel(entry.Level)

	// Build the basic log line
	var parts []string
	parts = append(parts, timestamp)
	parts = append(parts, level)

	// Add component if present
	if entry.Component != "" {
		if f.enableColors {
			parts = append(parts, fmt.Sprintf("\x1b[36m[%s]\x1b[0m", entry.Component))
		} else {
			parts = append(parts, fmt.Sprintf("[%s]", entry.Component))
		}
	}

	// Add operation ID if present
	if entry.OperationID != "" {
		parts = append(parts, fmt.Sprintf("op=%s", entry.OperationID))
	}

	// Add message
	parts = append(parts, entry.Message)

	// Format fields if present
	if len(entry.Fields) > 0 {
		fieldsStr := f.formatFields(entry.Fields)
		if fieldsStr != "" {
			parts = append(parts, fieldsStr)
		}
	}

	// Format error if present
	if entry.Error != nil {
		if f.enableColors {
			parts = append(parts, fmt.Sprintf("\x1b[31merror=%v\x1b[0m", entry.Error))
		} else {
			parts = append(parts, fmt.Sprintf("error=%v", entry.Error))
		}
	}

	line := strings.Join(parts, " ") + "\n"
	_, err := writer.Write([]byte(line))
	if err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// formatLevel formats the log level with appropriate color.
func (f *HumanFormatter) formatLevel(level LogLevel) string {
	levelStr := level.String()

	if !f.enableColors {
		return fmt.Sprintf("%-5s", levelStr)
	}

	switch level {
	case LogLevelDebug:
		return fmt.Sprintf("\x1b[37m%-5s\x1b[0m", levelStr) // Gray
	case LogLevelInfo:
		return fmt.Sprintf("\x1b[32m%-5s\x1b[0m", levelStr) // Green
	case LogLevelWarn:
		return fmt.Sprintf("\x1b[33m%-5s\x1b[0m", levelStr) // Yellow
	case LogLevelError:
		return fmt.Sprintf("\x1b[31m%-5s\x1b[0m", levelStr) // Red
	case LogLevelFatal:
		return fmt.Sprintf("\x1b[35m%-5s\x1b[0m", levelStr) // Magenta
	default:
		return fmt.Sprintf("%-5s", levelStr)
	}
}

// formatFields formats additional fields.
func (f *HumanFormatter) formatFields(fields map[string]any) string {
	if len(fields) == 0 {
		return ""
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		v := fields[key]
		switch val := v.(type) {
		case string:
			// quote strings for clarity; handles spaces/newlines safely
			parts = append(parts, fmt.Sprintf("%s=%q", key, val))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", key, v))
		}
	}

	if f.enableColors {
		return fmt.Sprintf("\x1b[90m%s\x1b[0m", strings.Join(parts, " "))
	}
	return strings.Join(parts, " ")
}
