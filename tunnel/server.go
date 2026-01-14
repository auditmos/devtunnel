package tunnel

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/auditmos/devtunnel/logging"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/acme/autocert"
)

type SharedBlob struct {
	ID         string
	Ciphertext []byte
	CreatedAt  int64
	ExpiresAt  int64
}

type BlobRepo interface {
	Save(blob *SharedBlob) error
	Get(id string) (*SharedBlob, error)
}

//go:embed templates/*.html
var serverTemplates embed.FS

type Server struct {
	addr     string
	domain   string
	upgrader websocket.Upgrader

	mu       sync.RWMutex
	sessions map[string]*Session

	httpServer  *http.Server
	httpsServer *http.Server
	listener    net.Listener
	tlsListener net.Listener

	blobRepo      BlobRepo
	templates     *template.Template
	certManager   *autocert.Manager
	enableHTTPS   bool
	certsDir      string
	readyCallback func()
	version       string
	rateLimiter   *RateLimiter
	logger        logging.Logger
}

type Session struct {
	Subdomain string
	PublicURL string
	Session   *yamux.Session
	ConnectedAt time.Time
}

type ServerConfig struct {
	Addr           string
	Domain         string
	BlobRepo       BlobRepo
	AutoDomain     bool
	EnableHTTPS    bool
	CertsDir       string
	Version        string
	RequestsPerMin int
	MaxConns       int
	Logger         logging.Logger
}

func NewServer(cfg ServerConfig) *Server {
	tmpl, _ := template.ParseFS(serverTemplates, "templates/*.html")

	domain := cfg.Domain
	if domain == "" && cfg.AutoDomain {
		if detected, err := AutoDetectDomain(); err == nil {
			domain = detected
		}
	}

	certsDir := cfg.CertsDir
	if certsDir == "" && cfg.EnableHTTPS {
		if home, err := os.UserHomeDir(); err == nil {
			certsDir = filepath.Join(home, ".devtunnel", "certs")
		}
	}

	ver := cfg.Version
	if ver == "" {
		ver = "dev"
	}

	reqPerMin := cfg.RequestsPerMin
	if reqPerMin <= 0 {
		reqPerMin = 60
	}
	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = 5
	}

	logger := cfg.Logger
	if logger == nil {
		logger = logging.NopLogger{}
	}

	s := &Server{
		addr:        cfg.Addr,
		domain:      domain,
		sessions:    make(map[string]*Session),
		blobRepo:    cfg.BlobRepo,
		templates:   tmpl,
		enableHTTPS: cfg.EnableHTTPS,
		certsDir:    certsDir,
		version:     ver,
		rateLimiter: NewRateLimiter(reqPerMin, maxConns),
		logger:      logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	if cfg.EnableHTTPS && domain != "" {
		if err := os.MkdirAll(certsDir, 0700); err != nil {
			logger.WithError(err).Warn("server", "config", "Failed to create certs dir")
		}
		s.certManager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(certsDir),
			HostPolicy: s.hostPolicy,
		}
	}

	return s
}

func (s *Server) hostPolicy(_ context.Context, host string) error {
	if s.domain == "" {
		return nil
	}
	if host == s.domain || strings.HasSuffix(host, "."+s.domain) {
		return nil
	}
	return fmt.Errorf("host %q not allowed", host)
}

func (s *Server) SetReadyCallback(cb func()) {
	s.readyCallback = cb
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", s.handleConnect)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/proxy/", s.handleProxy)
	mux.HandleFunc("/api/share", s.handleShare)
	mux.HandleFunc("/api/blob/", s.handleGetBlob)
	mux.HandleFunc("/api/rate-limits", s.handleRateLimits)
	mux.HandleFunc("/shared/", s.handleSharedView)
	mux.HandleFunc("/", s.handleSubdomainProxy)

	var handler http.Handler = mux
	if s.certManager != nil {
		handler = s.certManager.HTTPHandler(mux)
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("server listen: %w", err)
	}
	s.listener = ln

	s.httpServer = &http.Server{
		Handler: handler,
	}

	if s.readyCallback != nil {
		s.readyCallback()
	}

	log.Printf("Server listening on %s", s.addr)

	if s.enableHTTPS && s.certManager != nil {
		go s.startHTTPS(ctx, mux)
		log.Printf("Public URL: https://*.%s", s.domain)
	} else if s.domain != "" {
		log.Printf("Public URL: http://*.%s", s.domain)
	}

	go func() {
		<-ctx.Done()
		s.httpServer.Shutdown(context.Background())
		if s.httpsServer != nil {
			s.httpsServer.Shutdown(context.Background())
		}
	}()

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server serve: %w", err)
	}
	return nil
}

func (s *Server) startHTTPS(ctx context.Context, mux *http.ServeMux) {
	tlsConfig := &tls.Config{
		GetCertificate: s.certManager.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
	}

	tlsLn, err := tls.Listen("tcp", ":443", tlsConfig)
	if err != nil {
		log.Printf("HTTPS listen failed (fallback to HTTP): %v", err)
		return
	}
	s.tlsListener = tlsLn

	s.httpsServer = &http.Server{
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	log.Printf("HTTPS listening on :443")

	if err := s.httpsServer.Serve(tlsLn); err != nil && err != http.ErrServerClosed {
		log.Printf("HTTPS serve error: %v", err)
	}
}

func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Domain() string {
	return s.domain
}

type HealthResponse struct {
	OK      bool   `json:"ok"`
	Time    string `json:"time"`
	Version string `json:"version"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		OK:      true,
		Time:    time.Now().UTC().Format(time.RFC3339),
		Version: s.version,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}

	wsConn := NewWSConn(conn)

	cfg := yamux.DefaultConfig()
	cfg.KeepAliveInterval = 30 * time.Second
	cfg.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Server(wsConn, cfg)
	if err != nil {
		log.Printf("yamux server: %v", err)
		conn.Close()
		return
	}

	stream, err := session.Accept()
	if err != nil {
		log.Printf("accept handshake stream: %v", err)
		session.Close()
		return
	}

	var req HandshakeRequest
	dec := json.NewDecoder(stream)
	if err := dec.Decode(&req); err != nil {
		log.Printf("decode handshake: %v", err)
		stream.Close()
		session.Close()
		return
	}

	subdomain := generateSubdomain()
	if req.Subdomain != "" && s.isSubdomainAvailable(req.Subdomain) {
		subdomain = req.Subdomain
	}

	if s.rateLimiter != nil && !s.rateLimiter.AcquireConnection(subdomain) {
		log.Printf("connection limit exceeded for subdomain: %s", subdomain)
		resp := HandshakeResponse{
			Success: false,
			Error:   "connection limit exceeded",
		}
		json.NewEncoder(stream).Encode(&resp)
		stream.Close()
		session.Close()
		return
	}

	publicURL := fmt.Sprintf("http://%s.%s", subdomain, s.domain)
	if s.domain == "" {
		publicURL = fmt.Sprintf("http://localhost/%s", subdomain)
	}

	sess := &Session{
		Subdomain:   subdomain,
		PublicURL:   publicURL,
		Session:     session,
		ConnectedAt: time.Now(),
	}

	s.mu.Lock()
	s.sessions[subdomain] = sess
	s.mu.Unlock()

	resp := HandshakeResponse{
		Success:   true,
		Subdomain: subdomain,
		PublicURL: publicURL,
	}

	enc := json.NewEncoder(stream)
	if err := enc.Encode(&resp); err != nil {
		log.Printf("encode handshake response: %v", err)
		s.removeSession(subdomain)
		if s.rateLimiter != nil {
			s.rateLimiter.ReleaseConnection(subdomain)
		}
		stream.Close()
		session.Close()
		return
	}
	stream.Close()

	log.Printf("Client connected: %s -> %s", subdomain, publicURL)

	go s.monitorSession(sess)
}

func (s *Server) monitorSession(sess *Session) {
	<-sess.Session.CloseChan()
	s.removeSession(sess.Subdomain)
	if s.rateLimiter != nil {
		s.rateLimiter.ReleaseConnection(sess.Subdomain)
	}
	log.Printf("Client disconnected: %s", sess.Subdomain)
}

func (s *Server) removeSession(subdomain string) {
	s.mu.Lock()
	delete(s.sessions, subdomain)
	s.mu.Unlock()
}

func (s *Server) isSubdomainAvailable(subdomain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.sessions[subdomain]
	return !exists
}

func (s *Server) GetSession(subdomain string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[subdomain]
}

func (s *Server) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *Server) Close() error {
	var err error
	if s.httpServer != nil {
		err = s.httpServer.Close()
	}
	if s.httpsServer != nil {
		if e := s.httpsServer.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxy/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing subdomain", http.StatusBadRequest)
		return
	}

	subdomain := parts[0]
	targetPath := "/"
	if len(parts) > 1 {
		targetPath = "/" + parts[1]
	}

	if s.rateLimiter != nil {
		if ok, retryAfter := s.rateLimiter.AllowRequest(subdomain); !ok {
			WriteRateLimitExceeded(w, retryAfter)
			return
		}
	}

	sess := s.GetSession(subdomain)
	if sess == nil {
		http.Error(w, "tunnel not found", http.StatusBadGateway)
		return
	}

	s.proxyToTunnel(w, r, sess, targetPath)
}

func (s *Server) proxyToTunnel(w http.ResponseWriter, r *http.Request, sess *Session, targetPath string) {
	stream, err := sess.Session.Open()
	if err != nil {
		log.Printf("proxy open stream: %v", err)
		http.Error(w, "tunnel unavailable", http.StatusBadGateway)
		return
	}
	defer stream.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	reqFrame := RequestFrame{
		ID:      ulid.Make().String(),
		Method:  r.Method,
		URL:     targetPath,
		Headers: headers,
		Body:    body,
	}

	enc := json.NewEncoder(stream)
	if err := enc.Encode(&reqFrame); err != nil {
		log.Printf("proxy encode request: %v", err)
		http.Error(w, "tunnel write failed", http.StatusBadGateway)
		return
	}

	var respFrame ResponseFrame
	dec := json.NewDecoder(stream)
	if err := dec.Decode(&respFrame); err != nil {
		log.Printf("proxy decode response: %v", err)
		http.Error(w, "tunnel read failed", http.StatusBadGateway)
		return
	}

	for k, v := range respFrame.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(respFrame.StatusCode)
	w.Write(respFrame.Body)
}

func (s *Server) extractSubdomainFromHost(host string) string {
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.Split(host, ":")[0]
	if s.domain == "" {
		return ""
	}
	suffix := "." + s.domain
	if !strings.HasSuffix(host, suffix) {
		return ""
	}
	subdomain := strings.TrimSuffix(host, suffix)
	if strings.Contains(subdomain, ".") {
		return ""
	}
	return subdomain
}

func (s *Server) handleSubdomainProxy(w http.ResponseWriter, r *http.Request) {
	subdomain := s.extractSubdomainFromHost(r.Host)
	if subdomain == "" {
		http.NotFound(w, r)
		return
	}

	if s.rateLimiter != nil {
		if ok, retryAfter := s.rateLimiter.AllowRequest(subdomain); !ok {
			WriteRateLimitExceeded(w, retryAfter)
			return
		}
	}

	sess := s.GetSession(subdomain)
	if sess == nil {
		http.Error(w, "tunnel not found", http.StatusBadGateway)
		return
	}

	targetPath := r.URL.Path
	if targetPath == "" {
		targetPath = "/"
	}
	if r.URL.RawQuery != "" {
		targetPath += "?" + r.URL.RawQuery
	}

	s.proxyToTunnel(w, r, sess, targetPath)
}

type ShareRequest struct {
	Ciphertext string `json:"ciphertext"`
}

type ShareResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.blobRepo == nil {
		writeServerJSONError(w, "sharing not enabled", http.StatusServiceUnavailable)
		return
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeServerJSONError(w, "invalid json", http.StatusBadRequest)
		return
	}

	ciphertext, err := base64.StdEncoding.DecodeString(req.Ciphertext)
	if err != nil {
		writeServerJSONError(w, "invalid ciphertext encoding", http.StatusBadRequest)
		return
	}

	if len(ciphertext) > 10*1024*1024 {
		writeServerJSONError(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	blob := &SharedBlob{
		ID:         ulid.Make().String(),
		Ciphertext: ciphertext,
	}

	if err := s.blobRepo.Save(blob); err != nil {
		log.Printf("save blob: %v", err)
		writeServerJSONError(w, "failed to save", http.StatusInternalServerError)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ShareResponse{
		ID:  blob.ID,
		URL: fmt.Sprintf("%s://%s/shared/%s", scheme, r.Host, blob.ID),
	})
}

func (s *Server) handleGetBlob(w http.ResponseWriter, r *http.Request) {
	if s.blobRepo == nil {
		writeServerJSONError(w, "sharing not enabled", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/blob/")
	if id == "" {
		writeServerJSONError(w, "missing id", http.StatusBadRequest)
		return
	}

	blob, err := s.blobRepo.Get(id)
	if err != nil {
		log.Printf("get blob: %v", err)
		writeServerJSONError(w, "failed to get", http.StatusInternalServerError)
		return
	}
	if blob == nil {
		writeServerJSONError(w, "not found or expired", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ciphertext": base64.StdEncoding.EncodeToString(blob.Ciphertext),
	})
}

type SharedViewData struct {
	ID string
}

func (s *Server) handleSharedView(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/shared/")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.templates == nil {
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "shared.html", SharedViewData{ID: id}); err != nil {
		log.Printf("render shared: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func writeServerJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type RateLimitsResponse struct {
	RequestsPerMin     int `json:"requests_per_min"`
	MaxConcurrentConns int `json:"max_concurrent_conns"`
}

func (s *Server) handleRateLimits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqPerMin, maxConns := 60, 5
	if s.rateLimiter != nil {
		reqPerMin, maxConns = s.rateLimiter.GetLimits()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RateLimitsResponse{
		RequestsPerMin:     reqPerMin,
		MaxConcurrentConns: maxConns,
	})
}
