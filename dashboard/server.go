package dashboard

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/auditmos/devtunnel/crypto"
	"github.com/auditmos/devtunnel/storage"
	"github.com/oklog/ulid/v2"
)

//go:embed templates/*.html
var embeddedTemplates embed.FS

type Server struct {
	addr       string
	localAddr  string
	serverAddr string
	repo       storage.RequestRepo
	httpServer *http.Server
	templates  *template.Template
	httpClient *http.Client
}

type ServerConfig struct {
	Addr         string
	Repo         storage.RequestRepo
	OverridesDir string
	LocalAddr    string
	ServerAddr   string
}

func NewServer(cfg ServerConfig) (*Server, error) {
	localAddr := cfg.LocalAddr
	if localAddr == "" {
		localAddr = "localhost:3000"
	}

	s := &Server{
		addr:       cfg.Addr,
		localAddr:  localAddr,
		serverAddr: cfg.ServerAddr,
		repo:       cfg.Repo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	tmpl, err := loadTemplates(cfg.OverridesDir)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	s.templates = tmpl

	return s, nil
}

func loadTemplates(overridesDir string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
	}

	tmpl := template.New("").Funcs(funcMap)

	useOverrides := false
	if overridesDir != "" {
		if info, err := os.Stat(overridesDir); err == nil && info.IsDir() {
			useOverrides = true
		}
	}

	if useOverrides {
		err := filepath.WalkDir(overridesDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
				return err
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			relPath, _ := filepath.Rel(overridesDir, path)
			name := strings.TrimSuffix(relPath, ".html")
			_, parseErr := tmpl.New(name).Parse(string(content))
			return parseErr
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := fs.ReadDir(embeddedTemplates, "templates")
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
				continue
			}
			content, readErr := fs.ReadFile(embeddedTemplates, "templates/"+entry.Name())
			if readErr != nil {
				return nil, readErr
			}
			name := strings.TrimSuffix(entry.Name(), ".html")
			_, parseErr := tmpl.New(name).Parse(string(content))
			if parseErr != nil {
				return nil, parseErr
			}
		}
	}

	return tmpl, nil
}

func (s *Server) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/requests", s.handleAPIRequests)
	mux.HandleFunc("/api/replay/", s.handleReplay)
	mux.HandleFunc("/api/share/", s.handleShare)
	return mux
}

func (s *Server) testHandler() http.Handler {
	return s.buildMux()
}

func (s *Server) Start(ctx context.Context) error {
	mux := s.buildMux()

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

type RequestView struct {
	ID                       string
	Method                   string
	URL                      string
	StatusCode               int
	StatusClass              string
	DurationMs               int64
	TimeAgo                  string
	RequestHeaders           map[string]string
	RequestHeadersFormatted  string
	RequestBody              string
	ResponseHeaders          map[string]string
	ResponseHeadersFormatted string
	ResponseBody             string
}

type IndexData struct {
	Requests    []RequestView
	LastUpdated string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	requests, err := s.repo.ListAll(10)
	if err != nil {
		http.Error(w, "failed to load requests", http.StatusInternalServerError)
		return
	}

	views := make([]RequestView, len(requests))
	for i, req := range requests {
		views[i] = toRequestView(req)
	}

	data := IndexData{
		Requests:    views,
		LastUpdated: time.Now().Format("15:04:05"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := s.templates.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAPIRequests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func toRequestView(req *storage.Request) RequestView {
	return RequestView{
		ID:                       req.ID,
		Method:                   req.Method,
		URL:                      req.URL,
		StatusCode:               req.StatusCode,
		StatusClass:              statusClass(req.StatusCode),
		DurationMs:               req.DurationMs,
		TimeAgo:                  timeAgo(req.Timestamp),
		RequestHeaders:           req.RequestHeaders,
		RequestHeadersFormatted:  formatHeaders(req.RequestHeaders),
		RequestBody:              string(req.RequestBody),
		ResponseHeaders:          req.ResponseHeaders,
		ResponseHeadersFormatted: formatHeaders(req.ResponseHeaders),
		ResponseBody:             string(req.ResponseBody),
	}
}

func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "status-2xx"
	case code >= 300 && code < 400:
		return "status-3xx"
	case code >= 400 && code < 500:
		return "status-4xx"
	case code >= 500:
		return "status-5xx"
	default:
		return ""
	}
}

func timeAgo(timestamp int64) string {
	t := time.UnixMilli(timestamp)
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("Jan 2, 15:04")
	}
}

func formatHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return "(none)"
	}
	var sb strings.Builder
	for k, v := range headers {
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

type ReplayResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/replay/")
	if id == "" {
		writeJSONError(w, "missing request id", http.StatusBadRequest)
		return
	}

	storedReq, err := s.repo.Get(id)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("fetch request: %v", err), http.StatusInternalServerError)
		return
	}
	if storedReq == nil {
		writeJSONError(w, "request not found", http.StatusNotFound)
		return
	}

	start := time.Now()

	url := fmt.Sprintf("http://%s%s", s.localAddr, storedReq.URL)
	req, err := http.NewRequest(storedReq.Method, url, bytes.NewReader(storedReq.RequestBody))
	if err != nil {
		writeJSONError(w, fmt.Sprintf("create request: %v", err), http.StatusInternalServerError)
		return
	}

	for k, v := range storedReq.RequestHeaders {
		req.Header.Set(k, v)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("replay request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("read response: %v", err), http.StatusInternalServerError)
		return
	}

	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	newReq := &storage.Request{
		ID:              ulid.Make().String(),
		TunnelID:        storedReq.TunnelID,
		Timestamp:       time.Now().UnixMilli(),
		Method:          storedReq.Method,
		URL:             storedReq.URL,
		RequestHeaders:  storedReq.RequestHeaders,
		RequestBody:     storedReq.RequestBody,
		StatusCode:      resp.StatusCode,
		ResponseHeaders: respHeaders,
		ResponseBody:    respBody,
		DurationMs:      duration.Milliseconds(),
	}
	if saveErr := s.repo.Save(newReq); saveErr != nil {
		writeJSONError(w, fmt.Sprintf("save request: %v", saveErr), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ReplayResponse{
		StatusCode: resp.StatusCode,
		Headers:    respHeaders,
		Body:       string(respBody),
	})
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type ShareableRequest struct {
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestHeaders  map[string]string `json:"request_headers"`
	RequestBody     string            `json:"request_body"`
	StatusCode      int               `json:"status_code"`
	ResponseHeaders map[string]string `json:"response_headers"`
	ResponseBody    string            `json:"response_body"`
	DurationMs      int64             `json:"duration_ms"`
}

type ShareResponse struct {
	URL string `json:"url"`
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.serverAddr == "" {
		writeJSONError(w, "sharing not configured (no server address)", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/share/")
	if id == "" {
		writeJSONError(w, "missing request id", http.StatusBadRequest)
		return
	}

	storedReq, err := s.repo.Get(id)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("fetch request: %v", err), http.StatusInternalServerError)
		return
	}
	if storedReq == nil {
		writeJSONError(w, "request not found", http.StatusNotFound)
		return
	}

	shareable := ShareableRequest{
		Method:          storedReq.Method,
		URL:             storedReq.URL,
		RequestHeaders:  storedReq.RequestHeaders,
		RequestBody:     string(storedReq.RequestBody),
		StatusCode:      storedReq.StatusCode,
		ResponseHeaders: storedReq.ResponseHeaders,
		ResponseBody:    string(storedReq.ResponseBody),
		DurationMs:      storedReq.DurationMs,
	}

	plaintext, err := json.Marshal(shareable)
	if err != nil {
		writeJSONError(w, "marshal failed", http.StatusInternalServerError)
		return
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		writeJSONError(w, "key generation failed", http.StatusInternalServerError)
		return
	}

	ciphertext, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		writeJSONError(w, "encryption failed", http.StatusInternalServerError)
		return
	}

	serverURL := fmt.Sprintf("http://%s/api/share", s.serverAddr)
	reqBody, _ := json.Marshal(map[string]string{
		"ciphertext": base64.StdEncoding.EncodeToString(ciphertext),
	})

	resp, err := s.httpClient.Post(serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		writeJSONError(w, fmt.Sprintf("upload failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeJSONError(w, fmt.Sprintf("server error: %s", body), resp.StatusCode)
		return
	}

	var serverResp struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&serverResp); err != nil {
		writeJSONError(w, "invalid server response", http.StatusInternalServerError)
		return
	}

	shareURL := serverResp.URL + "#" + crypto.EncodeKey(key)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ShareResponse{URL: shareURL})
}
