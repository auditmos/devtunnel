package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

type RequestLog struct {
	Method          string
	URL             string
	RequestHeaders  map[string]string
	RequestBody     []byte
	StatusCode      int
	ResponseHeaders map[string]string
	ResponseBody    []byte
	DurationMs      int64
}

type RequestLogger interface {
	Log(log *RequestLog) error
}

type Client struct {
	serverAddr string
	localPort  string
	subdomain  string

	mu        sync.RWMutex
	session   *yamux.Session
	conn      *websocket.Conn
	publicURL string
	connected bool

	reconnect    bool
	maxBackoff   time.Duration
	onConnected  func(publicURL string)
	onDisconnect func(err error)
	logger       RequestLogger
}

type ClientConfig struct {
	ServerAddr string
	LocalPort  string
	Subdomain  string
}

func NewClient(cfg ClientConfig) *Client {
	return &Client{
		serverAddr: cfg.ServerAddr,
		localPort:  cfg.LocalPort,
		subdomain:  cfg.Subdomain,
		reconnect:  true,
		maxBackoff: 60 * time.Second,
	}
}

func (c *Client) OnConnected(fn func(publicURL string)) {
	c.onConnected = fn
}

func (c *Client) OnDisconnect(fn func(err error)) {
	c.onDisconnect = fn
}

func (c *Client) SetLogger(l RequestLogger) {
	c.logger = l
}

func (c *Client) Connect(ctx context.Context) error {
	return c.connectWithBackoff(ctx)
}

func (c *Client) connectWithBackoff(ctx context.Context) error {
	backoff := 1 * time.Second

	for {
		err := c.connect(ctx)
		if err == nil {
			return nil
		}

		if !c.reconnect {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		log.Printf("connect failed: %v, retry in %v", err, backoff)
		backoff *= 2
		if backoff > c.maxBackoff {
			backoff = c.maxBackoff
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	u := url.URL{Scheme: "ws", Host: c.serverAddr, Path: "/connect"}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	wsConn := NewWSConn(conn)

	cfg := yamux.DefaultConfig()
	cfg.KeepAliveInterval = 30 * time.Second
	cfg.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Client(wsConn, cfg)
	if err != nil {
		conn.Close()
		return fmt.Errorf("yamux client: %w", err)
	}

	stream, err := session.Open()
	if err != nil {
		session.Close()
		return fmt.Errorf("open handshake stream: %w", err)
	}

	req := HandshakeRequest{
		Version:   "1.0",
		Subdomain: c.subdomain,
	}

	enc := json.NewEncoder(stream)
	if err := enc.Encode(&req); err != nil {
		stream.Close()
		session.Close()
		return fmt.Errorf("encode handshake: %w", err)
	}

	var resp HandshakeResponse
	dec := json.NewDecoder(stream)
	if err := dec.Decode(&resp); err != nil {
		stream.Close()
		session.Close()
		return fmt.Errorf("decode handshake response: %w", err)
	}
	stream.Close()

	if !resp.Success {
		session.Close()
		return fmt.Errorf("handshake failed: %s", resp.Error)
	}

	c.mu.Lock()
	c.conn = conn
	c.session = session
	c.publicURL = resp.PublicURL
	c.subdomain = resp.Subdomain
	c.connected = true
	c.mu.Unlock()

	log.Printf("Connected: %s", resp.PublicURL)

	if c.onConnected != nil {
		c.onConnected(resp.PublicURL)
	}

	go c.handleRequests(ctx)
	go c.monitorConnection(ctx)

	return nil
}

func (c *Client) monitorConnection(ctx context.Context) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	select {
	case <-session.CloseChan():
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		if c.onDisconnect != nil {
			c.onDisconnect(fmt.Errorf("connection closed"))
		}

		if c.reconnect {
			log.Printf("Connection lost, reconnecting...")
			c.connectWithBackoff(ctx)
		}
	case <-ctx.Done():
		return
	}
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) PublicURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.publicURL
}

func (c *Client) Session() *yamux.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

func (c *Client) SetReconnect(v bool) {
	c.reconnect = v
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.reconnect = false
	c.connected = false

	if c.session != nil {
		c.session.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

func (c *Client) handleRequests(ctx context.Context) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		stream, err := session.Accept()
		if err != nil {
			return
		}

		go c.handleStream(stream)
	}
}

func (c *Client) handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	var req RequestFrame
	dec := json.NewDecoder(stream)
	if err := dec.Decode(&req); err != nil {
		log.Printf("decode request: %v", err)
		return
	}

	start := time.Now()

	localURL := fmt.Sprintf("http://127.0.0.1:%s%s", c.localPort, req.URL)
	httpReq, err := http.NewRequest(req.Method, localURL, bytes.NewReader(req.Body))
	if err != nil {
		log.Printf("create request: %v", err)
		c.sendError(stream, req.ID, http.StatusBadGateway)
		return
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("forward request: %v", err)
		c.sendError(stream, req.ID, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("read response: %v", err)
		c.sendError(stream, req.ID, http.StatusBadGateway)
		return
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	durationMs := time.Since(start).Milliseconds()

	if c.logger != nil {
		reqLog := &RequestLog{
			Method:          req.Method,
			URL:             req.URL,
			RequestHeaders:  req.Headers,
			RequestBody:     req.Body,
			StatusCode:      resp.StatusCode,
			ResponseHeaders: headers,
			ResponseBody:    body,
			DurationMs:      durationMs,
		}
		if err := c.logger.Log(reqLog); err != nil {
			log.Printf("log request: %v", err)
		}
	}

	respFrame := ResponseFrame{
		ID:         req.ID,
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}

	enc := json.NewEncoder(stream)
	if err := enc.Encode(&respFrame); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func (c *Client) sendError(stream io.Writer, id string, status int) {
	respFrame := ResponseFrame{
		ID:         id,
		StatusCode: status,
		Headers:    map[string]string{},
		Body:       []byte("tunnel error"),
	}
	enc := json.NewEncoder(stream)
	enc.Encode(&respFrame)
}
