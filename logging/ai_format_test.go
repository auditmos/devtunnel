package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LogSchema defines expected JSON log structure for AI parsing
type LogSchema struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component"`
	Action    string                 `json:"action"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ErrorType string                 `json:"error_type,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
}

func TestJSONL_ValidFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.Info("client", "connect", "test msg 1")
	logger.Debug("server", "proxy", "test msg 2")
	logger.WithFields(Fields{"key": "value"}).Warn("dashboard", "api", "test msg 3")

	scanner := bufio.NewScanner(&buf)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		var obj map[string]interface{}
		err := json.Unmarshal([]byte(line), &obj)
		require.NoError(t, err, "line %d should be valid JSON: %s", lineCount, line)
	}

	assert.Equal(t, 3, lineCount, "should have 3 log lines")
}

func TestJSONL_OneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.Info("test", "test", "message with\nnewline in content")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 1, "multiline message should be single JSON line")

	var obj map[string]interface{}
	err := json.Unmarshal([]byte(lines[0]), &obj)
	require.NoError(t, err)
	assert.Contains(t, obj["message"], "\n", "newline should be preserved in message field")
}

func TestJSON_RequiredFields(t *testing.T) {
	formatter := &JSONFormatter{}
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Component: "server",
		Action:    "start",
		Message:   "Server started",
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	required := []string{"timestamp", "level", "component", "action", "message"}
	for _, field := range required {
		assert.Contains(t, result, field, "required field %s missing", field)
		assert.NotEmpty(t, result[field], "required field %s should not be empty", field)
	}
}

func TestJSON_TimestampRFC3339(t *testing.T) {
	formatter := &JSONFormatter{}

	testTimes := []time.Time{
		time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Now().UTC(),
	}

	for _, ts := range testTimes {
		entry := LogEntry{
			Timestamp: ts,
			Level:     INFO,
			Component: "test",
			Action:    "test",
			Message:   "test",
		}

		data, err := formatter.Format(entry)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		tsStr, ok := result["timestamp"].(string)
		require.True(t, ok, "timestamp should be string")

		parsed, err := time.Parse(time.RFC3339, tsStr)
		require.NoError(t, err, "timestamp %s should be valid RFC3339", tsStr)
		assert.Equal(t, ts.Unix(), parsed.Unix())
	}
}

func TestJSON_LowercaseLevels(t *testing.T) {
	formatter := &JSONFormatter{}
	levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
	expected := []string{"debug", "info", "warn", "error"}

	for i, level := range levels {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     level,
			Component: "test",
			Action:    "test",
			Message:   "test",
		}

		data, err := formatter.Format(entry)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		assert.Equal(t, expected[i], result["level"], "level should be lowercase")
	}
}

func TestJSON_ConsistentStructure(t *testing.T) {
	formatter := &JSONFormatter{}

	entries := []LogEntry{
		{Timestamp: time.Now(), Level: DEBUG, Component: "client", Action: "connect", Message: "msg1"},
		{Timestamp: time.Now(), Level: INFO, Component: "server", Action: "proxy", Message: "msg2", TraceID: "trace1"},
		{Timestamp: time.Now(), Level: WARN, Component: "dashboard", Action: "api", Message: "msg3", Fields: Fields{"k": "v"}},
		{Timestamp: time.Now(), Level: ERROR, Component: "client", Action: "forward", Message: "msg4", Error: "err", ErrorType: "NetError"},
	}

	for _, entry := range entries {
		data, err := formatter.Format(entry)
		require.NoError(t, err)

		var schema LogSchema
		err = json.Unmarshal(data, &schema)
		require.NoError(t, err, "log should parse into consistent schema")

		assert.NotEmpty(t, schema.Timestamp)
		assert.NotEmpty(t, schema.Level)
		assert.NotEmpty(t, schema.Component)
		assert.NotEmpty(t, schema.Action)
		assert.NotEmpty(t, schema.Message)
	}
}

func TestJSON_FieldTyping(t *testing.T) {
	formatter := &JSONFormatter{}
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Component: "test",
		Action:    "test",
		Message:   "test",
		Fields: Fields{
			"string_field":  "value",
			"int_field":     123,
			"float_field":   3.14,
			"bool_field":    true,
			"duration_ms":   150,
			"nested_object": map[string]string{"nested_key": "nested_value"},
		},
	}

	data, err := formatter.Format(entry)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	fields, ok := result["fields"].(map[string]interface{})
	require.True(t, ok, "fields should be object")

	assert.IsType(t, "", fields["string_field"])
	assert.IsType(t, float64(0), fields["int_field"])
	assert.IsType(t, float64(0), fields["float_field"])
	assert.IsType(t, true, fields["bool_field"])
	assert.IsType(t, map[string]interface{}{}, fields["nested_object"])
}

func TestJSON_FilterByLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.Debug("test", "test", "debug msg")
	logger.Info("test", "test", "info msg")
	logger.Warn("test", "test", "warn msg")
	logger.Error("test", "test", "error msg")

	logs := parseJSONLogs(t, buf.String())
	assert.Len(t, logs, 4)

	filtered := filterByLevel(logs, "error")
	assert.Len(t, filtered, 1)
	assert.Equal(t, "error msg", filtered[0].Message)

	filtered = filterByLevel(logs, "warn")
	assert.Len(t, filtered, 2) // warn + error
}

func TestJSON_FilterByComponent(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.Info("client", "connect", "client msg")
	logger.Info("server", "proxy", "server msg")
	logger.Info("dashboard", "api", "dashboard msg")

	logs := parseJSONLogs(t, buf.String())

	filtered := filterByComponent(logs, "server")
	assert.Len(t, filtered, 1)
	assert.Equal(t, "server msg", filtered[0].Message)
}

func TestJSON_FilterByAction(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.Info("client", "connect", "connect msg")
	logger.Info("client", "forward", "forward msg")
	logger.Info("server", "connect", "server connect msg")

	logs := parseJSONLogs(t, buf.String())

	filtered := filterByAction(logs, "connect")
	assert.Len(t, filtered, 2)
}

func TestJSON_FilterByTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.WithTraceID("trace-abc").Info("client", "forward", "req 1")
	logger.WithTraceID("trace-abc").Info("server", "proxy", "req 1 proxied")
	logger.WithTraceID("trace-xyz").Info("client", "forward", "req 2")

	logs := parseJSONLogs(t, buf.String())

	filtered := filterByTraceID(logs, "trace-abc")
	assert.Len(t, filtered, 2)
	for _, log := range filtered {
		assert.Equal(t, "trace-abc", log.TraceID)
	}
}

func TestJSON_AIParseable(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{
		Output:    &buf,
		Formatter: &JSONFormatter{},
		Level:     DEBUG,
	})

	logger.WithFields(Fields{
		"subdomain":   "abc123",
		"method":      "POST",
		"path":        "/api/users",
		"status_code": 201,
		"duration_ms": 45,
	}).WithTraceID("01HXYZ123").Info("server", "proxy", "Request proxied")

	logger.WithError(WrapError("connect", errors.New("connection refused"))).
		WithFields(Fields{"retry_in": "5s"}).
		Error("client", "connect", "Connection failed")

	logs := parseJSONLogs(t, buf.String())
	require.Len(t, logs, 2)

	proxyLog := logs[0]
	assert.Equal(t, "server", proxyLog.Component)
	assert.Equal(t, "proxy", proxyLog.Action)
	assert.Equal(t, "Request proxied", proxyLog.Message)
	assert.Equal(t, "01HXYZ123", proxyLog.TraceID)
	assert.Equal(t, "abc123", proxyLog.Fields["subdomain"])
	assert.Equal(t, "POST", proxyLog.Fields["method"])
	assert.Equal(t, float64(201), proxyLog.Fields["status_code"])

	errorLog := logs[1]
	assert.Equal(t, "client", errorLog.Component)
	assert.Equal(t, "connect", errorLog.Action)
	assert.NotEmpty(t, errorLog.Error)
	assert.Equal(t, "5s", errorLog.Fields["retry_in"])
}

func TestJSON_NoExtraFields(t *testing.T) {
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

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	allowedKeys := map[string]bool{
		"timestamp":  true,
		"level":      true,
		"component":  true,
		"action":     true,
		"message":    true,
		"fields":     true,
		"error":      true,
		"error_type": true,
		"trace_id":   true,
	}

	for key := range result {
		assert.True(t, allowedKeys[key], "unexpected field in JSON: %s", key)
	}
}

func parseJSONLogs(t *testing.T, output string) []LogSchema {
	t.Helper()
	var logs []LogSchema
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var log LogSchema
		err := json.Unmarshal(scanner.Bytes(), &log)
		require.NoError(t, err)
		logs = append(logs, log)
	}
	return logs
}

func filterByLevel(logs []LogSchema, minLevel string) []LogSchema {
	levelOrder := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	minLevelNum := levelOrder[minLevel]

	var filtered []LogSchema
	for _, log := range logs {
		if levelOrder[log.Level] >= minLevelNum {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func filterByComponent(logs []LogSchema, component string) []LogSchema {
	var filtered []LogSchema
	for _, log := range logs {
		if log.Component == component {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func filterByAction(logs []LogSchema, action string) []LogSchema {
	var filtered []LogSchema
	for _, log := range logs {
		if log.Action == action {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func filterByTraceID(logs []LogSchema, traceID string) []LogSchema {
	var filtered []LogSchema
	for _, log := range logs {
		if log.TraceID == traceID {
			filtered = append(filtered, log)
		}
	}
	return filtered
}
