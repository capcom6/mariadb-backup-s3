package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
)

// Config holds the configuration for the logging system.
type Config struct {
	Level        LogLevel  `json:"level"`       // Level is the minimum log level to output
	Format       Format    `json:"format"`      // Format is the output format ("human" or "json")
	Output       io.Writer `json:"-"`           // Output is the output stream (empty for os.stdout)
	EnableColors bool      `json:"colors"`      // EnableColors enables colored output for human format
	TimeFormat   string    `json:"time_format"` // TimeFormat is the format for timestamps
}

// DefaultConfig returns the default logging configuration.
func DefaultConfig() Config {
	level := LogLevelInfo
	format := FormatHuman
	enableColors := true
	timeFormat := "2006-01-02 15:04:05.000"

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = LogLevelDebug
	case "info":
		level = LogLevelInfo
	case "warn":
		level = LogLevelWarn
	case "error":
		level = LogLevelError
	case "fatal":
		level = LogLevelFatal
	}

	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "json" {
		format = FormatJSON
	}

	var output io.Writer
	switch os.Getenv("LOG_OUTPUT") {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	case "":
		output = os.Stdout
	default:
		f, err := os.Create(os.Getenv("LOG_OUTPUT"))
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Failed to create log file %s: %v; falling back to stdout\n",
				os.Getenv("LOG_OUTPUT"),
				err,
			)
			output = os.Stdout
		} else {
			output = f
		}
	}

	if os.Getenv("NO_COLOR") != "" {
		enableColors = false
	}

	return Config{
		Level:        level,
		Format:       format,
		Output:       output,
		EnableColors: enableColors,
		TimeFormat:   timeFormat,
	}
}

// Validate validates the logging configuration.
func (c Config) Validate() error {
	if c.Level < LogLevelDebug || c.Level > LogLevelFatal {
		return fmt.Errorf("%w: invalid log level: %s", ErrValidationFailed, c.Level)
	}

	if c.Format != FormatHuman && c.Format != FormatJSON {
		return fmt.Errorf("%w: invalid format: %s (must be 'human' or 'json')", ErrValidationFailed, c.Format)
	}

	return nil
}
