package tunnel

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/oklog/ulid/v2"
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

	httpServer *http.Server
	listener   net.Listener

	blobRepo  BlobRepo
	templates *template.Template
}

type Session struct {
	Subdomain string
	PublicURL string
	Session   *yamux.Session
	ConnectedAt time.Time
}

type ServerConfig struct {
	Addr     string
	Domain   string
	BlobRepo BlobRepo
}

func NewServer(cfg ServerConfig) *Server {
	tmpl, _ := template.ParseFS(serverTemplates, "templates/*.html")

	s := &Server{
		addr:      cfg.Addr,
		domain:    cfg.Domain,
		sessions:  make(map[string]*Session),
		blobRepo:  cfg.BlobRepo,
		templates: tmpl,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", s.handleConnect)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/proxy/", s.handleProxy)
	mux.HandleFunc("/api/share", s.handleShare)
	mux.HandleFunc("/api/blob/", s.handleGetBlob)
	mux.HandleFunc("/shared/", s.handleSharedView)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("server listen: %w", err)
	}
	s.listener = ln

	s.httpServer = &http.Server{
		Handler: mux,
	}

	log.Printf("Server listening on %s", s.addr)

	go func() {
		<-ctx.Done()
		s.httpServer.Shutdown(context.Background())
	}()

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server serve: %w", err)
	}
	return nil
}

func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
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
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
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

	sess := s.GetSession(subdomain)
	if sess == nil {
		http.Error(w, "tunnel not found", http.StatusBadGateway)
		return
	}

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

	baseURL := s.domain
	if baseURL == "" {
		baseURL = r.Host
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ShareResponse{
		ID:  blob.ID,
		URL: fmt.Sprintf("http://%s/shared/%s", baseURL, blob.ID),
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
