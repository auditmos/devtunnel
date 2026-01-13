package storage

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/auditmos/devtunnel/tunnel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(&buf, false)

	input := &tunnel.RequestLog{
		Method:          "POST",
		URL:             "/webhook",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		RequestBody:     []byte(`{"event":"test"}`),
		StatusCode:      200,
		ResponseHeaders: map[string]string{"X-Response": "ok"},
		ResponseBody:    []byte(`{"status":"ok"}`),
		DurationMs:      42,
	}

	err := logger.Log(input)
	require.NoError(t, err)

	var entry JSONLogEntry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "POST", entry.Method)
	assert.Equal(t, "/webhook", entry.URL)
	assert.Equal(t, 200, entry.StatusCode)
	assert.Equal(t, int64(42), entry.DurationMs)
	assert.Equal(t, `{"event":"test"}`, entry.RequestBody)
	assert.Equal(t, `{"status":"ok"}`, entry.ResponseBody)
	assert.NotZero(t, entry.Timestamp)
}

func TestJSONLogger_SafeMode(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(&buf, true)

	input := &tunnel.RequestLog{
		Method:         "GET",
		URL:            "/api",
		RequestHeaders: map[string]string{"Authorization": "Bearer secret"},
		StatusCode:     200,
	}

	err := logger.Log(input)
	require.NoError(t, err)

	var entry JSONLogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	assert.Equal(t, "***", entry.RequestHeaders["Authorization"])
}

func TestMultiLogger_Log(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	l1 := NewJSONLogger(&buf1, false)
	l2 := NewJSONLogger(&buf2, false)
	multi := NewMultiLogger(l1, l2)

	input := &tunnel.RequestLog{
		Method:     "GET",
		URL:        "/test",
		StatusCode: 200,
	}

	err := multi.Log(input)
	require.NoError(t, err)

	assert.NotEmpty(t, buf1.Bytes())
	assert.NotEmpty(t, buf2.Bytes())
}
