package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_Defaults(t *testing.T) {
	logger := NewLogger(LoggerConfig{})
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.out)
	assert.NotNil(t, logger.formatter)
}

func TestStandardLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     WARN,
	})

	logger.Debug("test", "action", "debug message")
	logger.Info("test", "action", "info message")
	logger.Warn("test", "action", "warn message")
	logger.Error("test", "action", "error message")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	assert.Len(t, lines, 2)

	var entry1, entry2 map[string]interface{}
	require.NoError(t, json.Unmarshal(lines[0], &entry1))
	require.NoError(t, json.Unmarshal(lines[1], &entry2))

	assert.Equal(t, "warn", entry1["level"])
	assert.Equal(t, "error", entry2["level"])
}

func TestStandardLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	loggerWithFields := logger.WithFields(Fields{"user": "john", "request_id": "123"})
	loggerWithFields.Info("api", "request", "Processing request")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	fields, ok := result["fields"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "john", fields["user"])
	assert.Equal(t, "123", fields["request_id"])
}

func TestStandardLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	err := errors.New("connection failed")
	logger.WithError(err).Error("client", "connect", "Failed to connect")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "connection failed", result["error"])
}

func TestStandardLogger_WithError_Nil(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.WithError(nil).Info("test", "action", "message")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.NotContains(t, result, "error")
}

func TestStandardLogger_WithTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.WithTraceID("trace-abc-123").Info("server", "proxy", "Proxying request")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "trace-abc-123", result["trace_id"])
}

func TestStandardLogger_Chaining(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.
		WithTraceID("trace123").
		WithFields(Fields{"method": "GET"}).
		WithError(errors.New("timeout")).
		Error("proxy", "forward", "Request failed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "trace123", result["trace_id"])
	assert.Equal(t, "timeout", result["error"])

	fields := result["fields"].(map[string]interface{})
	assert.Equal(t, "GET", fields["method"])
}

func TestStandardLogger_Sanitize(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
		Sanitize:  true,
	})

	logger.
		WithFields(Fields{"password": "secret123", "user": "john"}).
		Info("auth", "login", "User logged in")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	fields := result["fields"].(map[string]interface{})
	assert.Equal(t, "[REDACTED]", fields["password"])
	assert.Equal(t, "john", fields["user"])
}

func TestStandardLogger_ThreadSafety(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(n int) {
			logger.Info("test", "action", "concurrent message")
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	assert.Len(t, lines, 100)
}

func TestNopLogger(t *testing.T) {
	logger := NopLogger{}

	logger.Debug("a", "b", "c")
	logger.Info("a", "b", "c")
	logger.Warn("a", "b", "c")
	logger.Error("a", "b", "c")

	assert.IsType(t, NopLogger{}, logger.WithFields(Fields{}))
	assert.IsType(t, NopLogger{}, logger.WithError(errors.New("test")))
	assert.IsType(t, NopLogger{}, logger.WithTraceID("abc"))
}
