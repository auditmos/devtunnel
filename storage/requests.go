package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

type Request struct {
	ID              string
	TunnelID        string
	Timestamp       int64
	Method          string
	URL             string
	RequestHeaders  map[string]string
	RequestBody     []byte
	StatusCode      int
	ResponseHeaders map[string]string
	ResponseBody    []byte
	DurationMs      int64
	CreatedAt       int64
}

type RequestRepo interface {
	Save(req *Request) error
	Get(id string) (*Request, error)
	List(tunnelID string, limit int) ([]*Request, error)
	ListAll(limit int) ([]*Request, error)
	Delete(id string) error
	Prune(olderThan time.Time) (int64, error)
}

type SQLiteRequestRepo struct {
	db *sql.DB
}

func NewSQLiteRequestRepo(db *sql.DB) *SQLiteRequestRepo {
	return &SQLiteRequestRepo{db: db}
}

func (r *SQLiteRequestRepo) Save(req *Request) error {
	if req.ID == "" {
		req.ID = ulid.Make().String()
	}
	if req.CreatedAt == 0 {
		req.CreatedAt = time.Now().UnixMilli()
	}

	reqHeaders, err := json.Marshal(req.RequestHeaders)
	if err != nil {
		return fmt.Errorf("marshal request headers: %w", err)
	}

	respHeaders, err := json.Marshal(req.ResponseHeaders)
	if err != nil {
		return fmt.Errorf("marshal response headers: %w", err)
	}

	_, err = r.db.Exec(`
		INSERT INTO requests (id, tunnel_id, timestamp, method, url, request_headers, request_body, status_code, response_headers, response_body, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.TunnelID, req.Timestamp, req.Method, req.URL, reqHeaders, req.RequestBody, req.StatusCode, respHeaders, req.ResponseBody, req.DurationMs, req.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert request: %w", err)
	}
	return nil
}

func (r *SQLiteRequestRepo) Get(id string) (*Request, error) {
	row := r.db.QueryRow(`
		SELECT id, tunnel_id, timestamp, method, url, request_headers, request_body, status_code, response_headers, response_body, duration_ms, created_at
		FROM requests WHERE id = ?
	`, id)

	req := &Request{}
	var reqHeaders, respHeaders []byte
	err := row.Scan(&req.ID, &req.TunnelID, &req.Timestamp, &req.Method, &req.URL, &reqHeaders, &req.RequestBody, &req.StatusCode, &respHeaders, &req.ResponseBody, &req.DurationMs, &req.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan request: %w", err)
	}

	if err := json.Unmarshal(reqHeaders, &req.RequestHeaders); err != nil {
		return nil, fmt.Errorf("unmarshal request headers: %w", err)
	}
	if len(respHeaders) > 0 {
		if err := json.Unmarshal(respHeaders, &req.ResponseHeaders); err != nil {
			return nil, fmt.Errorf("unmarshal response headers: %w", err)
		}
	}

	return req, nil
}

func (r *SQLiteRequestRepo) List(tunnelID string, limit int) ([]*Request, error) {
	rows, err := r.db.Query(`
		SELECT id, tunnel_id, timestamp, method, url, request_headers, request_body, status_code, response_headers, response_body, duration_ms, created_at
		FROM requests WHERE tunnel_id = ? ORDER BY timestamp DESC LIMIT ?
	`, tunnelID, limit)
	if err != nil {
		return nil, fmt.Errorf("query requests: %w", err)
	}
	defer rows.Close()

	return r.scanRows(rows)
}

func (r *SQLiteRequestRepo) ListAll(limit int) ([]*Request, error) {
	rows, err := r.db.Query(`
		SELECT id, tunnel_id, timestamp, method, url, request_headers, request_body, status_code, response_headers, response_body, duration_ms, created_at
		FROM requests ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query requests: %w", err)
	}
	defer rows.Close()

	return r.scanRows(rows)
}

func (r *SQLiteRequestRepo) scanRows(rows *sql.Rows) ([]*Request, error) {
	var requests []*Request
	for rows.Next() {
		req := &Request{}
		var reqHeaders, respHeaders []byte
		err := rows.Scan(&req.ID, &req.TunnelID, &req.Timestamp, &req.Method, &req.URL, &reqHeaders, &req.RequestBody, &req.StatusCode, &respHeaders, &req.ResponseBody, &req.DurationMs, &req.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}

		if err := json.Unmarshal(reqHeaders, &req.RequestHeaders); err != nil {
			return nil, fmt.Errorf("unmarshal request headers: %w", err)
		}
		if len(respHeaders) > 0 {
			if err := json.Unmarshal(respHeaders, &req.ResponseHeaders); err != nil {
				return nil, fmt.Errorf("unmarshal response headers: %w", err)
			}
		}

		requests = append(requests, req)
	}
	return requests, rows.Err()
}

func (r *SQLiteRequestRepo) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM requests WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	return nil
}

func (r *SQLiteRequestRepo) Prune(olderThan time.Time) (int64, error) {
	res, err := r.db.Exec("DELETE FROM requests WHERE timestamp < ?", olderThan.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("prune requests: %w", err)
	}
	return res.RowsAffected()
}
