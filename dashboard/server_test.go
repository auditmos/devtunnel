package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/auditmos/devtunnel/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRequestRepo struct {
	requests map[string]*storage.Request
}

func newMockRepo() *mockRequestRepo {
	return &mockRequestRepo{requests: make(map[string]*storage.Request)}
}

func (m *mockRequestRepo) Save(req *storage.Request) error {
	m.requests[req.ID] = req
	return nil
}

func (m *mockRequestRepo) Get(id string) (*storage.Request, error) {
	return m.requests[id], nil
}

func (m *mockRequestRepo) List(tunnelID string, limit int) ([]*storage.Request, error) {
	return nil, nil
}

func (m *mockRequestRepo) ListAll(limit int) ([]*storage.Request, error) {
	var result []*storage.Request
	for _, r := range m.requests {
		result = append(result, r)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockRequestRepo) Delete(id string) error {
	delete(m.requests, id)
	return nil
}

func (m *mockRequestRepo) Prune(olderThan time.Time) (int64, error) {
	return 0, nil
}

func TestReplay_Success(t *testing.T) {
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/webhook", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, `{"event":"test"}`, string(body))

		w.Header().Set("X-Response", "ok")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"received"}`))
	}))
	defer localServer.Close()

	repo := newMockRepo()
	repo.requests["req-001"] = &storage.Request{
		ID:             "req-001",
		Method:         "POST",
		URL:            "/webhook",
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
		RequestBody:    []byte(`{"event":"test"}`),
		Timestamp:      time.Now().UnixMilli(),
	}

	srv, err := NewServer(ServerConfig{
		Addr:      ":0",
		Repo:      repo,
		LocalAddr: localServer.URL[7:], // strip http://
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("POST", "/api/replay/req-001", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)

	var resp ReplayResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "ok", resp.Headers["X-Response"])
	assert.Equal(t, `{"status":"received"}`, resp.Body)
}

func TestReplay_RequestNotFound(t *testing.T) {
	repo := newMockRepo()

	srv, err := NewServer(ServerConfig{
		Addr:      ":0",
		Repo:      repo,
		LocalAddr: "localhost:3000",
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("POST", "/api/replay/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Equal(t, "request not found", resp["error"])
}

func TestReplay_LocalServerError(t *testing.T) {
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
		LocalAddr: "localhost:59999", // port likely not listening
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("POST", "/api/replay/req-002", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 502, rec.Code)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "replay request")
}

func TestReplay_SavesNewRequest(t *testing.T) {
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte("created"))
	}))
	defer localServer.Close()

	repo := newMockRepo()
	repo.requests["req-003"] = &storage.Request{
		ID:             "req-003",
		Method:         "POST",
		URL:            "/items",
		RequestHeaders: map[string]string{"Accept": "text/plain"},
		RequestBody:    []byte("item data"),
		TunnelID:       "tunnel-abc",
		Timestamp:      time.Now().UnixMilli(),
	}

	srv, err := NewServer(ServerConfig{
		Addr:      ":0",
		Repo:      repo,
		LocalAddr: localServer.URL[7:],
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("POST", "/api/replay/req-003", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)

	// verify new request was saved (should have 2 requests now)
	assert.Len(t, repo.requests, 2)

	var newReq *storage.Request
	for id, r := range repo.requests {
		if id != "req-003" {
			newReq = r
			break
		}
	}

	require.NotNil(t, newReq)
	assert.Equal(t, "POST", newReq.Method)
	assert.Equal(t, "/items", newReq.URL)
	assert.Equal(t, 201, newReq.StatusCode)
	assert.Equal(t, "tunnel-abc", newReq.TunnelID)
}

func TestAPIRequests_ReturnsJSON(t *testing.T) {
	repo := newMockRepo()
	now := time.Now().UnixMilli()
	repo.requests["req-001"] = &storage.Request{
		ID:              "req-001",
		TunnelID:        "tun-abc",
		Method:          "POST",
		URL:             "/webhook",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		RequestBody:     []byte(`{"event":"test"}`),
		StatusCode:      200,
		ResponseHeaders: map[string]string{"X-Response": "ok"},
		ResponseBody:    []byte(`{"status":"ok"}`),
		DurationMs:      42,
		Timestamp:       now,
	}

	srv, err := NewServer(ServerConfig{
		Addr: ":0",
		Repo: repo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/requests", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp APIRequestsResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp.Requests, 1)
	r := resp.Requests[0]
	assert.Equal(t, "req-001", r.ID)
	assert.Equal(t, "POST", r.Method)
	assert.Equal(t, "/webhook", r.URL)
	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, int64(42), r.DurationMs)
	assert.Equal(t, "application/json", r.RequestHeaders["Content-Type"])
	assert.Equal(t, `{"event":"test"}`, r.RequestBody)
}

func TestAPIRequests_Empty(t *testing.T) {
	repo := newMockRepo()

	srv, err := NewServer(ServerConfig{
		Addr: ":0",
		Repo: repo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/requests", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)

	var resp APIRequestsResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Empty(t, resp.Requests)
}

func TestAPIRequests_LimitParam(t *testing.T) {
	repo := newMockRepo()
	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		repo.requests[string(rune('a'+i))] = &storage.Request{
			ID:             string(rune('a' + i)),
			Method:         "GET",
			URL:            "/test",
			RequestHeaders: map[string]string{},
			Timestamp:      now - int64(i*1000),
		}
	}

	srv, err := NewServer(ServerConfig{
		Addr: ":0",
		Repo: repo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/requests?limit=2", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)

	var resp APIRequestsResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp.Requests, 2)
}

func TestDashboardReadySignal(t *testing.T) {
	repo := newMockRepo()

	srv, err := NewServer(ServerConfig{
		Addr: "127.0.0.1:0",
		Repo: repo,
	})
	require.NoError(t, err)

	ready := make(chan struct{})
	srv.SetReadyCallback(func() { close(ready) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Start(ctx)

	select {
	case <-ready:
		assert.NotEmpty(t, srv.Addr())
	case <-time.After(2 * time.Second):
		t.Fatal("dashboard did not signal ready")
	}
}

func TestDashboardPortConflict(t *testing.T) {
	repo := newMockRepo()

	srv1, err := NewServer(ServerConfig{
		Addr: "127.0.0.1:0",
		Repo: repo,
	})
	require.NoError(t, err)

	ready1 := make(chan struct{})
	srv1.SetReadyCallback(func() { close(ready1) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv1.Start(ctx)

	select {
	case <-ready1:
	case <-time.After(2 * time.Second):
		t.Fatal("first dashboard did not start")
	}

	addr := srv1.Addr()

	srv2, err := NewServer(ServerConfig{
		Addr: addr,
		Repo: repo,
	})
	require.NoError(t, err)

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

func setupScrubRuleRepo(t *testing.T) storage.ScrubRuleRepo {
	db, err := storage.OpenMemoryDB()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	repo := storage.NewSQLiteScrubRuleRepo(db)
	require.NoError(t, repo.Seed())
	return repo
}

func TestGetScrubRules_ReturnsSeededRules(t *testing.T) {
	scrubRepo := setupScrubRuleRepo(t)

	srv, err := NewServer(ServerConfig{
		Addr:          ":0",
		Repo:          newMockRepo(),
		ScrubRuleRepo: scrubRepo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/scrub-rules", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp APIScrubRulesResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Rules), 10)

	patterns := make(map[string]bool)
	for _, r := range resp.Rules {
		patterns[r.Pattern] = true
	}
	assert.True(t, patterns["authorization"])
	assert.True(t, patterns["cookie"])
}

func TestCreateScrubRule_Success(t *testing.T) {
	scrubRepo := setupScrubRuleRepo(t)

	srv, err := NewServer(ServerConfig{
		Addr:          ":0",
		Repo:          newMockRepo(),
		ScrubRuleRepo: scrubRepo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	body := bytes.NewBufferString(`{"pattern":"x-custom-secret"}`)
	req := httptest.NewRequest("POST", "/api/scrub-rules", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 201, rec.Code)

	var resp APIScrubRule
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "x-custom-secret", resp.Pattern)
	assert.NotZero(t, resp.CreatedAt)
}

func TestCreateScrubRule_EmptyPattern(t *testing.T) {
	scrubRepo := setupScrubRuleRepo(t)

	srv, err := NewServer(ServerConfig{
		Addr:          ":0",
		Repo:          newMockRepo(),
		ScrubRuleRepo: scrubRepo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	body := bytes.NewBufferString(`{"pattern":""}`)
	req := httptest.NewRequest("POST", "/api/scrub-rules", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 400, rec.Code)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "cannot be empty")
}

func TestCreateScrubRule_Duplicate(t *testing.T) {
	scrubRepo := setupScrubRuleRepo(t)

	srv, err := NewServer(ServerConfig{
		Addr:          ":0",
		Repo:          newMockRepo(),
		ScrubRuleRepo: scrubRepo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	body := bytes.NewBufferString(`{"pattern":"authorization"}`)
	req := httptest.NewRequest("POST", "/api/scrub-rules", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 409, rec.Code)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "already exists")
}

func TestDeleteScrubRule_Success(t *testing.T) {
	scrubRepo := setupScrubRuleRepo(t)

	rules, err := scrubRepo.GetAll()
	require.NoError(t, err)
	require.NotEmpty(t, rules)
	ruleID := rules[0].ID

	srv, err := NewServer(ServerConfig{
		Addr:          ":0",
		Repo:          newMockRepo(),
		ScrubRuleRepo: scrubRepo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("DELETE", "/api/scrub-rules/"+ruleID, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 204, rec.Code)

	rulesAfter, err := scrubRepo.GetAll()
	require.NoError(t, err)
	assert.Len(t, rulesAfter, len(rules)-1)
}

func TestDeleteScrubRule_NotFound(t *testing.T) {
	scrubRepo := setupScrubRuleRepo(t)

	srv, err := NewServer(ServerConfig{
		Addr:          ":0",
		Repo:          newMockRepo(),
		ScrubRuleRepo: scrubRepo,
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("DELETE", "/api/scrub-rules/nonexistent-id", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "not found")
}

func TestScrubRulesNotConfigured(t *testing.T) {
	srv, err := NewServer(ServerConfig{
		Addr: ":0",
		Repo: newMockRepo(),
	})
	require.NoError(t, err)

	handler := srv.testHandler()
	req := httptest.NewRequest("GET", "/api/scrub-rules", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, 503, rec.Code)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "not configured")
}
