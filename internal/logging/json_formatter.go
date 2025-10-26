package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// JSONFormatter formats log entries as JSON objects.
type JSONFormatter struct {
	// timeFormat is the format for timestamps in JSON output
	timeFormat string
}

// NewJSONFormatter creates a new JSON formatter.
func NewJSONFormatter(timeFormat string) *JSONFormatter {
	if timeFormat == "" {
		timeFormat = time.RFC3339Nano
	}
	return &JSONFormatter{
		timeFormat: timeFormat,
	}
}

// Name returns the formatter name.
func (f *JSONFormatter) Name() string {
	return "json"
}

// Format formats a log entry as JSON.
func (f *JSONFormatter) Format(entry *LogEntry, writer io.Writer) error {
	// Create a copy of the entry for JSON formatting
	jsonEntry := struct {
		Timestamp   string         `json:"timestamp"`
		Level       string         `json:"level"`
		Message     string         `json:"message"`
		Component   string         `json:"component,omitempty"`
		OperationID string         `json:"operation_id,omitempty"`
		Fields      map[string]any `json:"fields,omitempty"`
		Error       string         `json:"error,omitempty"`
	}{
		Timestamp:   entry.Timestamp.Format(f.timeFormat),
		Level:       entry.Level.String(),
		Message:     entry.Message,
		Fields:      entry.Fields,
		Component:   entry.Component,
		OperationID: entry.OperationID,
		Error: func() string {
			if entry.Error != nil {
				return entry.Error.Error()
			}
			return ""
		}(),
	}

	// Marshal to JSON and write
	data, err := json.Marshal(jsonEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry to JSON: %w", err)
	}

	_, err = writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write log entry to output: %w", err)
	}

	// Add newline for readability
	if _, err = writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}
	return nil
}
