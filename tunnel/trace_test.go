package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/auditmos/devtunnel/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceIDPropagationToLocalServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedTraceID string
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceID = r.Header.Get("X-Trace-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer localServer.Close()

	localPort := localServer.URL[len("http://127.0.0.1:"):]

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  localPort,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	assert.NotEmpty(t, receivedTraceID, "local server should receive X-Trace-ID header")
	assert.Len(t, receivedTraceID, 26, "trace_id should be ULID format (26 chars)")
}

func TestTraceIDInResponseHeader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer localServer.Close()

	localPort := localServer.URL[len("http://127.0.0.1:"):]

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  localPort,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	traceID := resp.Header.Get("X-Trace-ID")
	assert.NotEmpty(t, traceID, "response should have X-Trace-ID header")
	assert.Len(t, traceID, 26, "trace_id should be ULID format")
}

func TestTraceIDConsistencyAcrossLogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer localServer.Close()

	localPort := localServer.URL[len("http://127.0.0.1:"):]

	var serverBuf, clientBuf bytes.Buffer
	serverLogger := logging.NewLogger(logging.LoggerConfig{
		Output:    &serverBuf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})
	clientLogger := logging.NewLogger(logging.LoggerConfig{
		Output:    &clientBuf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv := NewServer(ServerConfig{
		Addr:   "127.0.0.1:0",
		Domain: "test.local",
		Logger: serverLogger,
	})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  localPort,
		Logger:     clientLogger,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	serverBuf.Reset()
	clientBuf.Reset()

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/api/test")
	require.NoError(t, err)
	responseTraceID := resp.Header.Get("X-Trace-ID")
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	serverTraceID := extractTraceIDFromLogs(t, serverBuf.String(), "proxy")
	clientTraceID := extractTraceIDFromLogs(t, clientBuf.String(), "forward")

	assert.NotEmpty(t, responseTraceID, "response should have trace_id")
	assert.NotEmpty(t, serverTraceID, "server logs should have trace_id")
	assert.NotEmpty(t, clientTraceID, "client logs should have trace_id")

	assert.Equal(t, responseTraceID, serverTraceID, "server log trace_id should match response")
	assert.Equal(t, responseTraceID, clientTraceID, "client log trace_id should match response")
}

func TestTraceIDPassthroughFromIncomingRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedTraceID string
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceID = r.Header.Get("X-Trace-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer localServer.Close()

	localPort := localServer.URL[len("http://127.0.0.1:"):]

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  localPort,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	incomingTraceID := "01HWABCD1234567890ABCDEF"
	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/proxy/"+sess.Subdomain+"/test", nil)
	req.Header.Set("X-Trace-ID", incomingTraceID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, incomingTraceID, receivedTraceID, "local server should receive original trace_id")
	assert.Equal(t, incomingTraceID, resp.Header.Get("X-Trace-ID"), "response should echo original trace_id")
}

func TestTraceIDFilterableLogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer localServer.Close()

	localPort := localServer.URL[len("http://127.0.0.1:"):]

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv := NewServer(ServerConfig{
		Addr:   "127.0.0.1:0",
		Domain: "test.local",
		Logger: logger,
	})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  localPort,
		Logger:     logger,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	buf.Reset()

	resp1, _ := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/req1")
	traceID1 := resp1.Header.Get("X-Trace-ID")
	resp1.Body.Close()

	resp2, _ := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/req2")
	traceID2 := resp2.Header.Get("X-Trace-ID")
	resp2.Body.Close()

	time.Sleep(50 * time.Millisecond)

	assert.NotEqual(t, traceID1, traceID2, "different requests should have different trace_ids")

	logsForTrace1 := filterLogsByTraceID(buf.String(), traceID1)
	logsForTrace2 := filterLogsByTraceID(buf.String(), traceID2)

	assert.GreaterOrEqual(t, len(logsForTrace1), 1, "should find logs for first request")
	assert.GreaterOrEqual(t, len(logsForTrace2), 1, "should find logs for second request")

	for _, log := range logsForTrace1 {
		assert.NotContains(t, log, traceID2, "trace1 logs should not contain trace2")
	}
}

func extractTraceIDFromLogs(t *testing.T, logs string, action string) string {
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["action"] == action {
			if fields, ok := entry["fields"].(map[string]interface{}); ok {
				if traceID, ok := fields["trace_id"].(string); ok {
					return traceID
				}
			}
			if traceID, ok := entry["trace_id"].(string); ok {
				return traceID
			}
		}
	}
	return ""
}

func filterLogsByTraceID(logs string, traceID string) []string {
	var result []string
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.Contains(line, traceID) {
			result = append(result, line)
		}
	}
	return result
}

func TestTraceIDInErrorLogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	localServer.Close()

	localPort := localServer.URL[len("http://127.0.0.1:"):]

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  localPort,
		Logger:     logger,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	buf.Reset()

	resp, _ := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/test")
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	foundErrorWithTraceID := false
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["level"] == "error" {
			if fields, ok := entry["fields"].(map[string]interface{}); ok {
				if _, hasTrace := fields["request_id"]; hasTrace {
					foundErrorWithTraceID = true
					break
				}
			}
		}
	}

	assert.True(t, foundErrorWithTraceID, "error logs should have request context")
}
