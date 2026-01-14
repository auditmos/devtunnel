package logging

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFormatter_Format(t *testing.T) {
	formatter := &JSONFormatter{}
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	entry := LogEntry{
		Timestamp: ts,
		Level:     INFO,
		Component: "client",
		Action:    "connect",
		Message:   "Connected to server",
		Fields:    Fields{"server": "example.com"},
		TraceID:   "abc123",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "2024-01-15T10:30:00Z", result["timestamp"])
	assert.Equal(t, "info", result["level"])
	assert.Equal(t, "client", result["component"])
	assert.Equal(t, "connect", result["action"])
	assert.Equal(t, "Connected to server", result["message"])
	assert.Equal(t, "abc123", result["trace_id"])

	fields, ok := result["fields"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "example.com", fields["server"])
}

func TestJSONFormatter_Format_WithError(t *testing.T) {
	formatter := &JSONFormatter{}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     ERROR,
		Component: "server",
		Action:    "proxy",
		Message:   "Proxy failed",
		Error:     "connection refused",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "connection refused", result["error"])
}

func TestJSONFormatter_Format_MinimalEntry(t *testing.T) {
	formatter := &JSONFormatter{}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     DEBUG,
		Component: "test",
		Action:    "test",
		Message:   "test message",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.NotContains(t, result, "fields")
	assert.NotContains(t, result, "error")
	assert.NotContains(t, result, "trace_id")
}

func TestJSONFormatter_OutputsJSONL(t *testing.T) {
	formatter := &JSONFormatter{}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Component: "test",
		Action:    "test",
		Message:   "test",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	assert.True(t, bytes.HasSuffix(data, []byte("\n")))
	assert.Equal(t, 1, bytes.Count(data, []byte("\n")))
}

func TestHumanFormatter_Format(t *testing.T) {
	var buf bytes.Buffer
	formatter := NewHumanFormatter(&buf)

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	entry := LogEntry{
		Timestamp: ts,
		Level:     INFO,
		Component: "client",
		Action:    "connect",
		Message:   "Connected",
		Fields:    Fields{"port": 3000},
		TraceID:   "trace123",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	output := string(data)
	assert.Contains(t, output, "10:30:00")
	assert.Contains(t, output, "info")
	assert.Contains(t, output, "[client]")
	assert.Contains(t, output, "connect:")
	assert.Contains(t, output, "Connected")
	assert.Contains(t, output, "port=3000")
	assert.Contains(t, output, "trace_id=trace123")
	assert.True(t, bytes.HasSuffix(data, []byte("\n")))
}

func TestHumanFormatter_Format_WithError(t *testing.T) {
	var buf bytes.Buffer
	formatter := NewHumanFormatter(&buf)

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     ERROR,
		Component: "server",
		Action:    "start",
		Message:   "Failed",
		Error:     "port in use",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	output := string(data)
	assert.Contains(t, output, "error=port in use")
}
