package tunnel

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientLogsConnectionAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		LocalPort:  "3000",
		Logger:     logger,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2)

	var connectAttempt map[string]interface{}
	require.NoError(t, json.Unmarshal(lines[0], &connectAttempt))

	assert.Equal(t, "client", connectAttempt["component"])
	assert.Equal(t, "connect", connectAttempt["action"])
	assert.Equal(t, "info", connectAttempt["level"])
	assert.Contains(t, connectAttempt["message"], "Connecting")

	fields := connectAttempt["fields"].(map[string]interface{})
	assert.NotEmpty(t, fields["server_addr"])
}

func TestClientLogsConnectionSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		LocalPort:  "3000",
		Logger:     logger,
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2)

	var connectSuccess map[string]interface{}
	require.NoError(t, json.Unmarshal(lines[1], &connectSuccess))

	assert.Equal(t, "client", connectSuccess["component"])
	assert.Equal(t, "connect", connectSuccess["action"])
	assert.Equal(t, "info", connectSuccess["level"])
	assert.Contains(t, connectSuccess["message"], "Connected")

	fields := connectSuccess["fields"].(map[string]interface{})
	assert.NotEmpty(t, fields["public_url"])
	assert.NotEmpty(t, fields["subdomain"])
}

func TestClientLogsRequestForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer localServer.Close()

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

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 1)

	var forwardLog map[string]interface{}
	require.NoError(t, json.Unmarshal(lines[0], &forwardLog))

	assert.Equal(t, "client", forwardLog["component"])
	assert.Equal(t, "forward", forwardLog["action"])
	assert.Equal(t, "info", forwardLog["level"])
	assert.Contains(t, forwardLog["message"], "Request forwarded")

	fields := forwardLog["fields"].(map[string]interface{})
	assert.Equal(t, "GET", fields["method"])
	assert.Equal(t, "/test", fields["url"])
	assert.NotNil(t, fields["status_code"])
	assert.NotNil(t, fields["duration_ms"])
}

func TestClientLogsRequestForwardingWithTraceID(t *testing.T) {
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

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/api/data")
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 1)

	var forwardLog map[string]interface{}
	require.NoError(t, json.Unmarshal(lines[0], &forwardLog))

	fields := forwardLog["fields"].(map[string]interface{})
	assert.NotEmpty(t, fields["request_id"])
}

func TestClientLogsConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	client := NewClient(ClientConfig{
		ServerAddr: "127.0.0.1:59999",
		LocalPort:  "3000",
		Logger:     logger,
	})
	client.SetReconnect(true)
	client.maxBackoff = 100 * time.Millisecond

	go client.Connect(ctx)

	time.Sleep(1500 * time.Millisecond)
	cancel()

	output := buf.String()
	assert.Contains(t, output, `"level":"error"`)
	assert.Contains(t, output, `"component":"client"`)
	assert.Contains(t, output, `"action":"connect"`)
	assert.Contains(t, output, "Connection failed")
	assert.Contains(t, output, "retry_in")
}

func TestClientLogsRequestError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("intentional crash")
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
		resp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"level":"error"`)
	assert.Contains(t, output, `"action":"forward"`)
	assert.Contains(t, output, "Failed to forward request")
}

func TestClientLogsDisconnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		LocalPort:  "3000",
		Logger:     logger,
	})

	err := client.Connect(ctx)
	require.NoError(t, err)

	buf.Reset()

	client.session.Close()

	time.Sleep(200 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"level":"warn"`)
	assert.Contains(t, output, `"action":"disconnect"`)
	assert.Contains(t, output, "Connection lost")
}

func TestClientLogsValidJSONL(t *testing.T) {
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

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/")
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
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
