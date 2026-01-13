package tunnel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	assert.NotEmpty(t, srv.Addr())

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestClientServerConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
	})
	client.SetReconnect(false)

	connected := make(chan string, 1)
	client.OnConnected(func(url string) {
		connected <- url
	})

	err := client.Connect(ctx)
	require.NoError(t, err)

	select {
	case url := <-connected:
		assert.Contains(t, url, "test.local")
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive connected callback")
	}

	assert.True(t, client.IsConnected())
	assert.NotEmpty(t, client.PublicURL())
	assert.Equal(t, 1, srv.SessionCount())

	client.Close()
	time.Sleep(100 * time.Millisecond)
	assert.False(t, client.IsConnected())
}

func TestClientReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)
	assert.True(t, client.IsConnected())

	firstURL := client.PublicURL()

	srv.Close()
	time.Sleep(100 * time.Millisecond)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	srv2 := NewServer(ServerConfig{Addr: srv.Addr(), Domain: "test.local"})
	go srv2.Start(ctx2)
	time.Sleep(50 * time.Millisecond)

	client2 := NewClient(ClientConfig{
		ServerAddr: srv2.Addr(),
		LocalPort:  "3000",
	})
	client2.SetReconnect(false)

	err = client2.Connect(ctx2)
	require.NoError(t, err)
	assert.True(t, client2.IsConnected())
	assert.NotEqual(t, firstURL, client2.PublicURL())
}

func TestMultipleClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = NewClient(ClientConfig{
			ServerAddr: srv.Addr(),
			LocalPort:  "3000",
		})
		clients[i].SetReconnect(false)
		err := clients[i].Connect(ctx)
		require.NoError(t, err)
	}

	assert.Equal(t, 3, srv.SessionCount())

	urls := make(map[string]bool)
	for _, c := range clients {
		urls[c.PublicURL()] = true
	}
	assert.Len(t, urls, 3)

	for _, c := range clients {
		c.Close()
	}
}

func TestServerLogsClientConnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, srv.SessionCount())
	sess := srv.GetSession(client.publicURL[len("http://"):len(client.publicURL)-len(".test.local")])
	if sess == nil {
		for sub := range srv.sessions {
			sess = srv.GetSession(sub)
			break
		}
	}
	require.NotNil(t, sess)
	assert.NotEmpty(t, sess.Subdomain)
}

func TestConnectionDropDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
	})
	client.SetReconnect(false)

	disconnected := make(chan error, 1)
	client.OnDisconnect(func(err error) {
		disconnected <- err
	})

	err := client.Connect(ctx)
	require.NoError(t, err)

	client.session.Close()

	select {
	case err := <-disconnected:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("did not detect disconnect")
	}
}

func TestSubdomainRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	client := NewClient(ClientConfig{
		ServerAddr: srv.Addr(),
		LocalPort:  "3000",
		Subdomain:  "myapp",
	})
	client.SetReconnect(false)

	err := client.Connect(ctx)
	require.NoError(t, err)

	assert.Contains(t, client.PublicURL(), "myapp")
}

func TestHealthEndpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	resp, err := (&http.Client{}).Get("http://" + srv.Addr() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
}

func TestHTTPForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from local"))
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

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/" + sess.Subdomain + "/test-path")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello from local", string(body))
	assert.Equal(t, "test-value", resp.Header.Get("X-Custom"))
}

func TestHTTPForwardingWithBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedBody []byte
	var receivedMethod string
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
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

	reqBody := strings.NewReader(`{"key":"value"}`)
	resp, err := http.Post("http://"+srv.Addr()+"/proxy/"+sess.Subdomain+"/api/data", "application/json", reqBody)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "POST", receivedMethod)
	assert.Equal(t, `{"key":"value"}`, string(receivedBody))
}

func TestHTTPForwardingPreservesHeaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedHeaders http.Header
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
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

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/proxy/"+sess.Subdomain+"/", nil)
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("X-Request-ID", "req-456")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "Bearer token123", receivedHeaders.Get("Authorization"))
	assert.Equal(t, "req-456", receivedHeaders.Get("X-Request-ID"))
}

func TestHTTPForwardingNoSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + srv.Addr() + "/proxy/nonexistent/path")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}

func TestRequestLogging(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response", "logged")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response body"))
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

	var logged []*RequestLog
	logger := &mockLogger{logs: &logged}
	client.SetLogger(logger)

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	sess := getFirstSession(srv)
	require.NotNil(t, sess)

	req, _ := http.NewRequest("POST", "http://"+srv.Addr()+"/proxy/"+sess.Subdomain+"/webhook", strings.NewReader(`{"event":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	require.Len(t, logged, 1)
	log := logged[0]
	assert.Equal(t, "POST", log.Method)
	assert.Equal(t, "/webhook", log.URL)
	assert.Equal(t, "application/json", log.RequestHeaders["Content-Type"])
	assert.Equal(t, "Bearer secret", log.RequestHeaders["Authorization"])
	assert.Equal(t, []byte(`{"event":"test"}`), log.RequestBody)
	assert.Equal(t, 200, log.StatusCode)
	assert.Equal(t, "logged", log.ResponseHeaders["X-Response"])
	assert.Equal(t, []byte("response body"), log.ResponseBody)
	assert.GreaterOrEqual(t, log.DurationMs, int64(0))
}

type mockLogger struct {
	logs *[]*RequestLog
}

func (m *mockLogger) Log(l *RequestLog) error {
	*m.logs = append(*m.logs, l)
	return nil
}

func getFirstSession(srv *Server) *Session {
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	for _, sess := range srv.sessions {
		return sess
	}
	return nil
}

func TestServerReadySignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	srv.SetReadyCallback(func() { close(ready) })

	go srv.Start(ctx)

	select {
	case <-ready:
		assert.NotEmpty(t, srv.Addr())
	case <-time.After(2 * time.Second):
		t.Fatal("server did not signal ready")
	}
}

func TestServerPortConflict(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv1 := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	ready1 := make(chan struct{})
	srv1.SetReadyCallback(func() { close(ready1) })

	go srv1.Start(ctx)

	select {
	case <-ready1:
	case <-time.After(2 * time.Second):
		t.Fatal("first server did not start")
	}

	addr := srv1.Addr()

	srv2 := NewServer(ServerConfig{Addr: addr, Domain: "test.local"})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv2.Start(ctx)
	}()

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "address already in use")
	case <-time.After(2 * time.Second):
		t.Fatal("port conflict not detected")
	}
}

func TestServerReadyBeforeURLPrinted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})

	addrReady := make(chan string, 1)
	srv.SetReadyCallback(func() {
		addrReady <- srv.Addr()
	})

	go srv.Start(ctx)

	select {
	case addr := <-addrReady:
		assert.NotEmpty(t, addr)
		assert.NotEqual(t, ":0", addr)
	case <-time.After(2 * time.Second):
		t.Fatal("ready callback not invoked")
	}
}

func TestSubdomainHostRouting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Routed-Via", "subdomain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("subdomain routed"))
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

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/test-path", nil)
	req.Host = sess.Subdomain + ".test.local"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "subdomain routed", string(body))
	assert.Equal(t, "subdomain", resp.Header.Get("X-Routed-Via"))
}

func TestSubdomainHostRoutingWithQueryString(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedPath string
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RequestURI()
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

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/api/data?foo=bar&baz=qux", nil)
	req.Host = sess.Subdomain + ".test.local"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/api/data?foo=bar&baz=qux", receivedPath)
}

func TestSubdomainHostRoutingInvalidHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/", nil)
	req.Host = "subdomain.wrong.domain"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSubdomainHostRoutingMissingSubdomain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/", nil)
	req.Host = "test.local"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSubdomainHostRoutingNestedSubdomain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/", nil)
	req.Host = "nested.subdomain.test.local"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSubdomainHostRoutingNoTunnel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Domain: "test.local"})
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/", nil)
	req.Host = "nonexistent.test.local"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}

func TestSubdomainHostRoutingWithPort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
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

	req, _ := http.NewRequest("GET", "http://"+srv.Addr()+"/", nil)
	req.Host = sess.Subdomain + ".test.local:8080"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
}
