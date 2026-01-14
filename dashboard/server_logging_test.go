package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/auditmos/devtunnel/logging"
	"github.com/auditmos/devtunnel/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardLogsStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv, err := NewServer(ServerConfig{
		Addr:   "127.0.0.1:0",
		Repo:   newMockRepo(),
		Logger: logger,
	})
	require.NoError(t, err)

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"component":"dashboard"`)
	assert.Contains(t, output, `"action":"start"`)
	assert.Contains(t, output, `"level":"info"`)
	assert.Contains(t, output, "Dashboard started")
	assert.Contains(t, output, "addr")
}

func TestDashboardLogsAPIRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv, err := NewServer(ServerConfig{
		Addr:   ":0",
		Repo:   newMockRepo(),
		Logger: logger,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/requests", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)

	output := buf.String()
	assert.Contains(t, output, `"component":"dashboard"`)
	assert.Contains(t, output, `"action":"api"`)
	assert.Contains(t, output, "Request received")
	assert.Contains(t, output, "method")
	assert.Contains(t, output, "path")
	assert.Contains(t, output, "trace_id")
}

func TestDashboardLogsReplaySuccess(t *testing.T) {
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer localServer.Close()

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	repo := newMockRepo()
	repo.requests["req-001"] = &storage.Request{
		ID:             "req-001",
		Method:         "POST",
		URL:            "/webhook",
		RequestHeaders: map[string]string{},
		Timestamp:      time.Now().UnixMilli(),
	}

	srv, err := NewServer(ServerConfig{
		Addr:      ":0",
		Repo:      repo,
		LocalAddr: localServer.URL[7:],
		Logger:    logger,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("POST", "/api/replay/req-001", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)

	output := buf.String()
	assert.Contains(t, output, `"component":"dashboard"`)
	assert.Contains(t, output, `"action":"replay"`)
	assert.Contains(t, output, "Request replayed")
	assert.Contains(t, output, "request_id")
	assert.Contains(t, output, "status_code")
	assert.Contains(t, output, "duration_ms")
	assert.Contains(t, output, "trace_id")
}

func TestDashboardLogsReplayError(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	repo := newMockRepo()
	repo.requests["req-002"] = &storage.Request{
		ID:             "req-002",
		Method:         "GET",
		URL:            "/test",
		RequestHeaders: map[string]string{},
		Timestamp:      time.Now().UnixMilli(),
	}

	srv, err := NewServer(ServerConfig{
		Addr:      ":0",
		Repo:      repo,
		LocalAddr: "localhost:59999",
		Logger:    logger,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("POST", "/api/replay/req-002", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 502, rec.Code)

	output := buf.String()
	assert.Contains(t, output, `"level":"error"`)
	assert.Contains(t, output, `"component":"dashboard"`)
	assert.Contains(t, output, `"action":"replay"`)
	assert.Contains(t, output, "Replay failed")
	assert.Contains(t, output, "error")
	assert.Contains(t, output, "trace_id")
}

func TestDashboardLogsValidJSONL(t *testing.T) {
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer localServer.Close()

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	repo := newMockRepo()
	repo.requests["req-003"] = &storage.Request{
		ID:             "req-003",
		Method:         "GET",
		URL:            "/",
		RequestHeaders: map[string]string{},
		Timestamp:      time.Now().UnixMilli(),
	}

	srv, err := NewServer(ServerConfig{
		Addr:      ":0",
		Repo:      repo,
		LocalAddr: localServer.URL[7:],
		Logger:    logger,
	})
	require.NoError(t, err)

	handler := srv.testHandler()

	req1 := httptest.NewRequest("GET", "/api/requests", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest("POST", "/api/replay/req-003", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		err := json.Unmarshal([]byte(line), &entry)
		assert.NoError(t, err, "line %d should be valid JSON: %s", i, line)

		assert.NotEmpty(t, entry["timestamp"], "line %d should have timestamp", i)
		assert.NotEmpty(t, entry["level"], "line %d should have level", i)
		assert.NotEmpty(t, entry["component"], "line %d should have component", i)
		assert.NotEmpty(t, entry["action"], "line %d should have action", i)
		assert.NotEmpty(t, entry["message"], "line %d should have message", i)
	}
}

func TestDashboardLogsTraceIDPresent(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv, err := NewServer(ServerConfig{
		Addr:   ":0",
		Repo:   newMockRepo(),
		Logger: logger,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/requests", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		err := json.Unmarshal([]byte(line), &entry)
		require.NoError(t, err)

		fields, ok := entry["fields"].(map[string]interface{})
		if ok {
			assert.NotEmpty(t, fields["trace_id"], "trace_id should be present")
		}
	}
}
