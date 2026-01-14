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

func TestServerLogsStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"component":"server"`)
	assert.Contains(t, output, `"action":"start"`)
	assert.Contains(t, output, `"level":"info"`)
	assert.Contains(t, output, "Server listening")
	assert.Contains(t, output, "addr")
	assert.Contains(t, output, "test.local")
}

func TestServerLogsClientConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	buf.Reset()

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"component":"server"`)
	assert.Contains(t, output, `"action":"connect"`)
	assert.Contains(t, output, "Client connected")
	assert.Contains(t, output, "subdomain")
	assert.Contains(t, output, "public_url")
}

func TestServerLogsClientDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)

	buf.Reset()

	client.Close()
	time.Sleep(100 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"component":"server"`)
	assert.Contains(t, output, `"action":"disconnect"`)
	assert.Contains(t, output, "Client disconnected")
	assert.Contains(t, output, "subdomain")
}

func TestServerLogsProxyRequest(t *testing.T) {
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

	srv := NewServer(ServerConfig{
		Addr:   "127.0.0.1:0",
		Domain: "test.local",
		Logger: logger,
	})

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

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

	buf.Reset()

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/api/test")
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"component":"server"`)
	assert.Contains(t, output, `"action":"proxy"`)
	assert.Contains(t, output, "Request proxied")
	assert.Contains(t, output, "subdomain")
	assert.Contains(t, output, "method")
	assert.Contains(t, output, "path")
	assert.Contains(t, output, "trace_id")
}

func TestServerLogsProxyErrorTunnelNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	buf.Reset()

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/nonexistent/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	resp.Body.Close()
}

type mockBlobRepo struct {
	blobs map[string]*SharedBlob
}

func (m *mockBlobRepo) Save(blob *SharedBlob) error {
	if m.blobs == nil {
		m.blobs = make(map[string]*SharedBlob)
	}
	m.blobs[blob.ID] = blob
	return nil
}

func (m *mockBlobRepo) Get(id string) (*SharedBlob, error) {
	return m.blobs[id], nil
}

func TestServerLogsBlobSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	logger := logging.NewLogger(logging.LoggerConfig{
		Output:    &buf,
		Formatter: &logging.JSONFormatter{},
		Level:     logging.DEBUG,
	})

	srv := NewServer(ServerConfig{
		Addr:     "127.0.0.1:0",
		Domain:   "test.local",
		Logger:   logger,
		BlobRepo: &mockBlobRepo{},
	})

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	buf.Reset()

	body := `{"ciphertext":"dGVzdA=="}`
	resp, err := http.Post("http://"+srv.Addr()+"/api/share", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"component":"server"`)
	assert.Contains(t, output, `"action":"blob"`)
	assert.Contains(t, output, "Blob saved")
	assert.Contains(t, output, "blob_id")
	assert.Contains(t, output, "size")
}

func TestServerLogsValidJSONL(t *testing.T) {
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

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

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

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/")
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

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

func TestServerLogsWebSocketUpgradeError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)
	<-ready

	buf.Reset()

	resp, _ := http.Get("http://" + srv.Addr() + "/connect")
	if resp != nil {
		resp.Body.Close()
	}

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, `"level":"error"`)
	assert.Contains(t, output, `"component":"server"`)
	assert.Contains(t, output, `"action":"websocket"`)
	assert.Contains(t, output, "Upgrade failed")
}

