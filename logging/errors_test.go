package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextError_Error(t *testing.T) {
	inner := errors.New("connection refused")
	err := WrapError("websocket dial", inner)

	assert.Equal(t, "websocket dial: connection refused", err.Error())
}

func TestContextError_Unwrap(t *testing.T) {
	inner := errors.New("connection refused")
	err := WrapError("websocket dial", inner)

	assert.True(t, errors.Is(err, inner))
}

func TestContextError_Type(t *testing.T) {
	inner := errors.New("something went wrong")
	err := WrapErrorWithType("open stream", inner, "StreamError")

	assert.Equal(t, "StreamError", err.Type())
}

func TestContextError_TypeInferred(t *testing.T) {
	inner := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}
	err := WrapError("connect", inner)

	assert.Equal(t, "OpError", err.Type())
}

func TestWrapError_Nil(t *testing.T) {
	err := WrapError("test", nil)
	assert.Nil(t, err)
}

func TestErrorType_Standard(t *testing.T) {
	err := errors.New("generic error")
	assert.Equal(t, "errorString", ErrorType(err))
}

func TestErrorType_Context(t *testing.T) {
	inner := errors.New("boom")
	err := WrapErrorWithType("op", inner, "CustomType")
	assert.Equal(t, "CustomType", ErrorType(err))
}

func TestErrorType_Nil(t *testing.T) {
	assert.Equal(t, "", ErrorType(nil))
}

type customError struct {
	msg string
}

func (e *customError) Error() string { return e.msg }
func (e *customError) Type() string  { return "CustomError" }

func TestErrorType_TypedError(t *testing.T) {
	err := &customError{msg: "custom"}
	assert.Equal(t, "CustomError", ErrorType(err))
}

func TestLogger_WithError_IncludesErrorType(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	err := WrapErrorWithType("connect", errors.New("refused"), "ConnectionError")
	logger.WithError(err).Error("client", "connect", "Failed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "connect: refused", result["error"])
	assert.Equal(t, "ConnectionError", result["error_type"])
}

func TestLogger_WithError_NetError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	logger.WithError(netErr).Error("client", "connect", "Failed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "OpError", result["error_type"])
}

func TestLogger_ErrorChaining_PreservesType(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	err := WrapErrorWithType("db", errors.New("timeout"), "DBError")
	logger.
		WithTraceID("trace123").
		WithError(err).
		WithFields(Fields{"query": "SELECT"}).
		Error("storage", "query", "Failed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "trace123", result["trace_id"])
	assert.Equal(t, "DBError", result["error_type"])
	assert.Equal(t, "db: timeout", result["error"])
}

func TestLogger_ErrorWithFullContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	err := fmt.Errorf("open stream: %w", errors.New("yamux session closed"))
	logger.
		WithTraceID("ULID123").
		WithFields(Fields{
			"subdomain":  "abc",
			"request_id": "req456",
			"method":     "POST",
		}).
		WithError(err).
		Error("server", "proxy", "Proxy failed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "error", result["level"])
	assert.Equal(t, "server", result["component"])
	assert.Equal(t, "proxy", result["action"])
	assert.Equal(t, "Proxy failed", result["message"])
	assert.Equal(t, "open stream: yamux session closed", result["error"])
	assert.Equal(t, "ULID123", result["trace_id"])

	fields := result["fields"].(map[string]interface{})
	assert.Equal(t, "abc", fields["subdomain"])
	assert.Equal(t, "req456", fields["request_id"])
	assert.Equal(t, "POST", fields["method"])
}

func TestLogger_SanitizeDoesNotLeakSensitiveErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
		Sanitize:  true,
	})

	logger.
		WithFields(Fields{
			"password": "secret123",
			"token":    "bearer-xxx",
			"user":     "john",
		}).
		WithError(errors.New("auth failed")).
		Error("auth", "login", "Failed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	fields := result["fields"].(map[string]interface{})
	assert.Equal(t, "[REDACTED]", fields["password"])
	assert.Equal(t, "[REDACTED]", fields["token"])
	assert.Equal(t, "john", fields["user"])
}

func TestLogger_JSONOutput_ValidAndParseable(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	err := WrapErrorWithType("connect", errors.New("refused"), "NetworkError")
	logger.WithError(err).WithTraceID("trace1").Error("client", "connect", "Failed")

	logger.WithFields(Fields{"status": 500}).Error("server", "proxy", "Error")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	assert.Len(t, lines, 2)

	for _, line := range lines {
		var entry map[string]interface{}
		require.NoError(t, json.Unmarshal(line, &entry), "JSON must be valid")

		assert.Contains(t, entry, "timestamp")
		assert.Contains(t, entry, "level")
		assert.Contains(t, entry, "component")
		assert.Contains(t, entry, "action")
		assert.Contains(t, entry, "message")
	}
}

func TestHumanFormatter_IncludesErrorType(t *testing.T) {
	formatter := &HumanFormatter{colorEnabled: false}
	entry := LogEntry{
		Level:     ERROR,
		Component: "client",
		Action:    "connect",
		Message:   "Failed",
		Error:     "connection refused",
		ErrorType: "ConnectionError",
	}

	output, err := formatter.Format(entry)
	require.NoError(t, err)

	assert.Contains(t, string(output), "error=connection refused")
	assert.Contains(t, string(output), "error_type=ConnectionError")
}
