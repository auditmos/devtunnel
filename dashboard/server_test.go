package dashboard

import (
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
