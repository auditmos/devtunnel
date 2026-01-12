package storage

import (
	"testing"
	"time"

	"github.com/auditmos/devtunnel/tunnel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenMemoryDB(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM requests").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestRequestRepo_SaveAndGet(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)

	req := &Request{
		TunnelID:        "tunnel-123",
		Timestamp:       time.Now().UnixMilli(),
		Method:          "POST",
		URL:             "/api/webhook",
		RequestHeaders:  map[string]string{"Content-Type": "application/json", "Authorization": "Bearer secret"},
		RequestBody:     []byte(`{"event":"test"}`),
		StatusCode:      200,
		ResponseHeaders: map[string]string{"X-Request-Id": "abc123"},
		ResponseBody:    []byte(`{"ok":true}`),
		DurationMs:      42,
	}

	err = repo.Save(req)
	require.NoError(t, err)
	assert.NotEmpty(t, req.ID)

	got, err := repo.Get(req.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, req.ID, got.ID)
	assert.Equal(t, req.TunnelID, got.TunnelID)
	assert.Equal(t, req.Method, got.Method)
	assert.Equal(t, req.URL, got.URL)
	assert.Equal(t, req.RequestHeaders, got.RequestHeaders)
	assert.Equal(t, req.RequestBody, got.RequestBody)
	assert.Equal(t, req.StatusCode, got.StatusCode)
	assert.Equal(t, req.ResponseHeaders, got.ResponseHeaders)
	assert.Equal(t, req.ResponseBody, got.ResponseBody)
	assert.Equal(t, req.DurationMs, got.DurationMs)
}

func TestRequestRepo_GetNotFound(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)

	got, err := repo.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRequestRepo_List(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)

	tunnelID := "tunnel-abc"
	for i := 0; i < 5; i++ {
		req := &Request{
			TunnelID:       tunnelID,
			Timestamp:      time.Now().UnixMilli() + int64(i),
			Method:         "GET",
			URL:            "/test",
			RequestHeaders: map[string]string{},
		}
		err := repo.Save(req)
		require.NoError(t, err)
	}

	otherReq := &Request{
		TunnelID:       "other-tunnel",
		Timestamp:      time.Now().UnixMilli(),
		Method:         "GET",
		URL:            "/other",
		RequestHeaders: map[string]string{},
	}
	err = repo.Save(otherReq)
	require.NoError(t, err)

	requests, err := repo.List(tunnelID, 10)
	require.NoError(t, err)
	assert.Len(t, requests, 5)

	for i := 1; i < len(requests); i++ {
		assert.GreaterOrEqual(t, requests[i-1].Timestamp, requests[i].Timestamp)
	}
}

func TestRequestRepo_ListAll(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)

	for i := 0; i < 15; i++ {
		req := &Request{
			TunnelID:       "tunnel-" + string(rune('a'+i%3)),
			Timestamp:      time.Now().UnixMilli() + int64(i),
			Method:         "GET",
			URL:            "/test",
			RequestHeaders: map[string]string{},
		}
		err := repo.Save(req)
		require.NoError(t, err)
	}

	requests, err := repo.ListAll(10)
	require.NoError(t, err)
	assert.Len(t, requests, 10)
}

func TestRequestRepo_Delete(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)

	req := &Request{
		TunnelID:       "tunnel-123",
		Timestamp:      time.Now().UnixMilli(),
		Method:         "GET",
		URL:            "/test",
		RequestHeaders: map[string]string{},
	}
	err = repo.Save(req)
	require.NoError(t, err)

	err = repo.Delete(req.ID)
	require.NoError(t, err)

	got, err := repo.Get(req.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRequestRepo_Prune(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)

	oldTime := time.Now().Add(-48 * time.Hour).UnixMilli()
	newTime := time.Now().UnixMilli()

	oldReq := &Request{
		TunnelID:       "tunnel-123",
		Timestamp:      oldTime,
		Method:         "GET",
		URL:            "/old",
		RequestHeaders: map[string]string{},
	}
	err = repo.Save(oldReq)
	require.NoError(t, err)

	newReq := &Request{
		TunnelID:       "tunnel-123",
		Timestamp:      newTime,
		Method:         "GET",
		URL:            "/new",
		RequestHeaders: map[string]string{},
	}
	err = repo.Save(newReq)
	require.NoError(t, err)

	cutoff := time.Now().Add(-24 * time.Hour)
	deleted, err := repo.Prune(cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	old, err := repo.Get(oldReq.ID)
	require.NoError(t, err)
	assert.Nil(t, old)

	new, err := repo.Get(newReq.ID)
	require.NoError(t, err)
	assert.NotNil(t, new)
}

func TestDBFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/devtunnel.db"

	db, err := OpenDB(dbPath)
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)
	req := &Request{
		TunnelID:       "tunnel-123",
		Timestamp:      time.Now().UnixMilli(),
		Method:         "GET",
		URL:            "/test",
		RequestHeaders: map[string]string{},
	}
	err = repo.Save(req)
	require.NoError(t, err)

	db.Close()

	db2, err := OpenDB(dbPath)
	require.NoError(t, err)
	defer db2.Close()

	repo2 := NewSQLiteRequestRepo(db2)
	got, err := repo2.Get(req.ID)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, req.ID, got.ID)
}

func TestDBLogger_SafeModeScrubsHeaders(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)
	logger := NewDBLogger(repo, "tunnel-123", true)

	reqLog := &tunnel.RequestLog{
		Method:          "POST",
		URL:             "/api/webhook",
		RequestHeaders:  map[string]string{"Authorization": "Bearer secret-token", "Content-Type": "application/json"},
		RequestBody:     []byte(`{"event":"test"}`),
		StatusCode:      200,
		ResponseHeaders: map[string]string{"Set-Cookie": "session=abc123", "Content-Type": "application/json"},
		ResponseBody:    []byte(`{"ok":true}`),
		DurationMs:      42,
	}

	err = logger.Log(reqLog)
	require.NoError(t, err)

	requests, err := repo.ListAll(1)
	require.NoError(t, err)
	require.Len(t, requests, 1)

	saved := requests[0]
	assert.Equal(t, "***", saved.RequestHeaders["Authorization"])
	assert.Equal(t, "application/json", saved.RequestHeaders["Content-Type"])
	assert.Equal(t, "***", saved.ResponseHeaders["Set-Cookie"])
	assert.Equal(t, "application/json", saved.ResponseHeaders["Content-Type"])
}

func TestDBLogger_NoSafeMode_PreservesHeaders(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteRequestRepo(db)
	logger := NewDBLogger(repo, "tunnel-123", false)

	reqLog := &tunnel.RequestLog{
		Method:          "POST",
		URL:             "/api/webhook",
		RequestHeaders:  map[string]string{"Authorization": "Bearer secret-token"},
		RequestBody:     []byte(`{}`),
		StatusCode:      200,
		ResponseHeaders: map[string]string{},
		ResponseBody:    []byte(`{}`),
		DurationMs:      10,
	}

	err = logger.Log(reqLog)
	require.NoError(t, err)

	requests, err := repo.ListAll(1)
	require.NoError(t, err)
	require.Len(t, requests, 1)

	saved := requests[0]
	assert.Equal(t, "Bearer secret-token", saved.RequestHeaders["Authorization"])
}

func TestBlobRepo_SaveAndGet(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitBlobSchema(db)
	require.NoError(t, err)

	repo := NewSQLiteBlobRepo(db)

	blob := &SharedBlob{
		Ciphertext: []byte("encrypted data here"),
	}

	err = repo.Save(blob)
	require.NoError(t, err)
	assert.NotEmpty(t, blob.ID)
	assert.NotZero(t, blob.CreatedAt)
	assert.NotZero(t, blob.ExpiresAt)

	got, err := repo.Get(blob.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, blob.ID, got.ID)
	assert.Equal(t, blob.Ciphertext, got.Ciphertext)
}

func TestBlobRepo_GetExpired(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitBlobSchema(db)
	require.NoError(t, err)

	repo := NewSQLiteBlobRepo(db)

	blob := &SharedBlob{
		Ciphertext: []byte("old data"),
		ExpiresAt:  time.Now().Add(-1 * time.Hour).UnixMilli(),
	}

	err = repo.Save(blob)
	require.NoError(t, err)

	got, err := repo.Get(blob.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBlobRepo_GetNotFound(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitBlobSchema(db)
	require.NoError(t, err)

	repo := NewSQLiteBlobRepo(db)

	got, err := repo.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBlobRepo_Delete(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitBlobSchema(db)
	require.NoError(t, err)

	repo := NewSQLiteBlobRepo(db)

	blob := &SharedBlob{
		Ciphertext: []byte("to delete"),
	}

	err = repo.Save(blob)
	require.NoError(t, err)

	err = repo.Delete(blob.ID)
	require.NoError(t, err)

	got, err := repo.Get(blob.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBlobRepo_Prune(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitBlobSchema(db)
	require.NoError(t, err)

	repo := NewSQLiteBlobRepo(db)

	expiredBlob := &SharedBlob{
		Ciphertext: []byte("old"),
		ExpiresAt:  time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	err = repo.Save(expiredBlob)
	require.NoError(t, err)

	validBlob := &SharedBlob{
		Ciphertext: []byte("fresh"),
		ExpiresAt:  time.Now().Add(24 * time.Hour).UnixMilli(),
	}
	err = repo.Save(validBlob)
	require.NoError(t, err)

	deleted, err := repo.Prune()
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	got, err := repo.Get(validBlob.ID)
	require.NoError(t, err)
	assert.NotNil(t, got)
}
