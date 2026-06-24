package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
)

func newBufferedLogger(t *testing.T, cfg logging.Config) (logging.Logger, *bytes.Buffer) {
	t.Helper()
	cfg.Output = &bytes.Buffer{}
	cfg.TimeFormat = "2006-01-02 15:04:05.000"
	logger, err := logging.New(cfg)
	require.NoError(t, err)
	return logger, cfg.Output.(*bytes.Buffer)
}

func TestNew_DefaultConfig(t *testing.T) {
	cfg := logging.Config{
		Level:        logging.LogLevelDebug,
		Format:       logging.FormatJSON,
		Output:       &bytes.Buffer{},
		EnableColors: false,
		TimeFormat:   "2006-01-02 15:04:05.000",
	}
	_, err := logging.New(cfg)
	assert.NoError(t, err)
}

func TestNew_InvalidFormat(t *testing.T) {
	cfg := logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       "invalid",
		Output:       &bytes.Buffer{},
		EnableColors: false,
		TimeFormat:   "",
	}
	_, err := logging.New(cfg)
	assert.Error(t, err)
}

func TestNew_InvalidLevel(t *testing.T) {
	cfg := logging.Config{
		Level:        logging.LogLevel(99),
		Format:       logging.FormatHuman,
		Output:       &bytes.Buffer{},
		EnableColors: false,
		TimeFormat:   "",
	}
	_, err := logging.New(cfg)
	assert.Error(t, err)
}

func TestLogger_Debug_BelowLevel(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})
	logger.Debug(context.Background(), "should not appear")
	assert.Empty(t, buf.String())
}

func TestLogger_Info_AtLevel(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})
	logger.Info(context.Background(), "info message")
	assert.Contains(t, buf.String(), "info message")
}

func TestLogger_Warn_AtLevel(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelWarn,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})
	logger.Warn(context.Background(), "warn message")
	assert.Contains(t, buf.String(), "warn message")

	logger.Info(context.Background(), "should not appear")
	assert.NotContains(t, buf.String(), "should not appear")
}

func TestLogger_Error_WithError(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelError,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})
	logger.Error(context.Background(), "error occurred", assert.AnError)
	output := buf.String()
	assert.Contains(t, output, "error occurred")
	assert.Contains(t, output, assert.AnError.Error())
}

func TestLogger_SetLevel(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelError,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	logger.Info(context.Background(), "should not appear")
	assert.Empty(t, buf.String())

	logger.SetLevel(logging.LogLevelInfo)
	logger.Info(context.Background(), "now it appears")
	assert.Contains(t, buf.String(), "now it appears")
}

func TestLogger_WithContext_Component(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	componentLogger := logger.WithContext("mycomponent", "op123")
	componentLogger.Info(context.Background(), "test")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "mycomponent", result["component"])
	assert.Equal(t, "op123", result["operation_id"])
}

func TestLogger_WithContext_Fields(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	fieldsLogger := logger.WithContext("comp", "op", logging.Fields{"key1": "val1", "key2": 42})
	fieldsLogger.Info(context.Background(), "with fields")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	assert.Equal(t, "val1", fields["key1"])
	assert.InDelta(t, 42, fields["key2"], 0)
}

func TestLogger_ContextEnrichment(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	ctx := logging.WithComponent(context.Background(), "ctxcomponent")
	ctx = logging.WithOperationID(ctx, "ctxop")
	logger.Info(ctx, "context enriched")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "ctxcomponent", result["component"])
	assert.Equal(t, "ctxop", result["operation_id"])
}

func TestLogger_ContextFields(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	ctx := logging.WithFields(context.Background(), logging.Fields{"ctxfield": "ctxval"})
	logger.Info(ctx, "with context fields")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	assert.Equal(t, "ctxval", fields["ctxfield"])
}

func TestLogger_WithContext_MergesFields(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	baseLogger := logger.WithContext("base", "op", logging.Fields{"basekey": "baseval"})
	ctx := logging.WithFields(context.Background(), logging.Fields{"ctxkey": "ctxval"})
	baseLogger.Info(ctx, "merged fields", logging.Fields{"callkey": "callval"})

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	assert.Equal(t, "baseval", fields["basekey"])
	assert.Equal(t, "ctxval", fields["ctxkey"])
	assert.Equal(t, "callval", fields["callkey"])
}

func TestLogger_WithContext_OverwritesFields(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	baseLogger := logger.WithContext("base", "op", logging.Fields{"key": "baseval"})
	ctx := logging.WithFields(context.Background(), logging.Fields{"key": "ctxval"})
	baseLogger.Info(ctx, "overwrite", logging.Fields{"key": "callval"})

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	assert.Equal(t, "callval", fields["key"], "call-time fields should have highest priority")
}

func TestLogger_Flush_NoFlusher(t *testing.T) {
	logger, _ := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})
	err := logger.Flush()
	assert.NoError(t, err)
}

func TestLogger_Close_NoCloser(t *testing.T) {
	logger, _ := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})
	err := logger.Close()
	assert.NoError(t, err)
}

func TestHumanFormatter_Output(t *testing.T) {
	formatter := logging.NewHumanFormatter(false, "2006-01-02 15:04:05.000")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelInfo,
		Message:     "hello world",
		Component:   "",
		OperationID: "",
		Fields:      nil,
		Error:       nil,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "INFO")
	assert.Contains(t, output, "hello world")
	assert.NotContains(t, output, "\x1b[")
}

func TestHumanFormatter_Colors(t *testing.T) {
	formatter := logging.NewHumanFormatter(true, "2006-01-02 15:04:05.000")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelInfo,
		Message:     "colored",
		Component:   "test",
		OperationID: "",
		Fields:      nil,
		Error:       assert.AnError,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "\x1b[")
	assert.Contains(t, output, "INFO")
	assert.Contains(t, output, "colored")
	assert.Contains(t, output, "test")
	assert.Contains(t, output, assert.AnError.Error())
}

func TestHumanFormatter_NoColors(t *testing.T) {
	formatter := logging.NewHumanFormatter(false, "2006-01-02 15:04:05.000")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelError,
		Message:     "no colors",
		Component:   "",
		OperationID: "",
		Fields:      nil,
		Error:       nil,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "\x1b[")
	assert.Contains(t, output, "ERROR")
}

func TestHumanFormatter_Fields(t *testing.T) {
	formatter := logging.NewHumanFormatter(false, "2006-01-02 15:04:05.000")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelInfo,
		Message:     "with fields",
		Component:   "",
		OperationID: "",
		Fields:      logging.Fields{"key1": "val1", "key2": 42},
		Error:       nil,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "key1")
	assert.Contains(t, output, "val1")
	assert.Contains(t, output, "key2")
}

func TestHumanFormatter_ComponentOperationID(t *testing.T) {
	formatter := logging.NewHumanFormatter(false, "2006-01-02 15:04:05.000")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelInfo,
		Message:     "test",
		Component:   "mycomp",
		OperationID: "op456",
		Fields:      nil,
		Error:       nil,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[mycomp]")
	assert.Contains(t, output, "op=op456")
}

func TestJSONFormatter_Output(t *testing.T) {
	formatter := logging.NewJSONFormatter("")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelInfo,
		Message:     "json test",
		Component:   "",
		OperationID: "",
		Fields:      nil,
		Error:       nil,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "INFO", result["level"])
	assert.Equal(t, "json test", result["message"])
}

func TestJSONFormatter_ErrorField(t *testing.T) {
	formatter := logging.NewJSONFormatter("")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelError,
		Message:     "error test",
		Component:   "",
		OperationID: "",
		Fields:      nil,
		Error:       assert.AnError,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "ERROR", result["level"])
	assert.Contains(t, result["error"], assert.AnError.Error())
}

func TestJSONFormatter_AllFields(t *testing.T) {
	formatter := logging.NewJSONFormatter("")
	entry := &logging.LogEntry{
		Timestamp:   fixedTime(),
		Level:       logging.LogLevelWarn,
		Message:     "full entry",
		Component:   "comp1",
		OperationID: "op1",
		Fields:      logging.Fields{"key": "val"},
		Error:       nil,
	}

	var buf bytes.Buffer
	err := formatter.Format(entry, &buf)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "WARN", result["level"])
	assert.Equal(t, "comp1", result["component"])
	assert.Equal(t, "op1", result["operation_id"])
}

func TestGenerateOperationID(t *testing.T) {
	id := logging.GenerateOperationID("testcomp")
	assert.Contains(t, id, "testcomp-")
}

func TestGenerateOperationID_EmptyComponent(t *testing.T) {
	id := logging.GenerateOperationID("")
	assert.Contains(t, id, "unknown-")
}

func TestDefaultConfig_EnvVars(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("NO_COLOR", "1")

	cfg := logging.DefaultConfig()
	assert.Equal(t, logging.LogLevelDebug, cfg.Level)
	assert.Equal(t, logging.Format("json"), cfg.Format)
	assert.False(t, cfg.EnableColors)
}

func TestDefaultConfig_DefaultValues(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("NO_COLOR", "")

	cfg := logging.DefaultConfig()
	assert.Equal(t, logging.LogLevelInfo, cfg.Level)
	assert.Equal(t, logging.FormatHuman, cfg.Format)
	assert.True(t, cfg.EnableColors)
}

func TestWithLogger_GetLogger(t *testing.T) {
	logger, _ := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	ctx := logging.WithLogger(context.Background(), logger)
	got := logging.GetLogger(ctx)
	assert.Equal(t, logger, got)
}

func TestGetLogger_NoLogger(t *testing.T) {
	got := logging.GetLogger(context.Background())
	assert.Nil(t, got)
}

func TestWithComponent_GetComponent(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	ctx := logging.WithComponent(context.Background(), "testcomp")
	ctx = logging.WithOperationID(ctx, "testop")
	logger.Info(ctx, "context test")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "testcomp", result["component"])
	assert.Equal(t, "testop", result["operation_id"])
}

func TestWithFields_MergesFields(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	ctx := logging.WithFields(context.Background(), logging.Fields{"key1": "val1"})
	ctx = logging.WithFields(ctx, logging.Fields{"key2": "val2"})
	logger.Info(ctx, "merged")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	assert.Equal(t, "val1", fields["key1"])
	assert.Equal(t, "val2", fields["key2"])
}

func TestLogger_Error_WithoutError(t *testing.T) {
	logger, buf := newBufferedLogger(t, logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		EnableColors: false,
		Output:       nil,
		TimeFormat:   "",
	})

	logger.Info(context.Background(), "no error")
	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	_, hasError := result["error"]
	assert.False(t, hasError, "error key should be absent when no error provided")
}

func TestLogger_OutputNotNil(t *testing.T) {
	cfg := logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       logging.FormatJSON,
		Output:       nil,
		EnableColors: false,
		TimeFormat:   "",
	}
	logger, err := logging.New(cfg)
	require.NoError(t, err)
	logger.Info(context.Background(), "should not panic")
}

func TestValidateConfig_Valid(t *testing.T) {
	err := logging.Config{
		Level:        logging.LogLevelWarn,
		Format:       logging.FormatHuman,
		Output:       nil,
		EnableColors: false,
		TimeFormat:   "",
	}.Validate()
	assert.NoError(t, err)
}

func TestValidateConfig_InvalidLevel(t *testing.T) {
	err := logging.Config{
		Level:        logging.LogLevel(99),
		Format:       logging.FormatHuman,
		Output:       nil,
		EnableColors: false,
		TimeFormat:   "",
	}.Validate()
	assert.Error(t, err)
}

func TestValidateConfig_InvalidFormat(t *testing.T) {
	err := logging.Config{
		Level:        logging.LogLevelInfo,
		Format:       "bad",
		Output:       nil,
		EnableColors: false,
		TimeFormat:   "",
	}.Validate()
	assert.Error(t, err)
}

func fixedTime() time.Time {
	t, err := time.Parse("2006-01-02 15:04:05.000", "2024-01-15 10:30:00.000")
	if err != nil {
		panic(err)
	}
	return t
}
