package storage

import (
	"database/sql"
	"fmt"
	"time"
)

type Tunnel struct {
	ID        string
	Subdomain string
	ServerURL string
	StartedAt int64
	EndedAt   int64
	Status    string
}

type TunnelRepo interface {
	Save(t *Tunnel) error
	Get(id string) (*Tunnel, error)
	UpdateStatus(id string, status string, endedAt int64) error
	ListActive() ([]*Tunnel, error)
}

type SQLiteTunnelRepo struct {
	db *sql.DB
}

func NewSQLiteTunnelRepo(db *sql.DB) *SQLiteTunnelRepo {
	return &SQLiteTunnelRepo{db: db}
}

func (r *SQLiteTunnelRepo) Save(t *Tunnel) error {
	if t.StartedAt == 0 {
		t.StartedAt = time.Now().UnixMilli()
	}
	if t.Status == "" {
		t.Status = "active"
	}

	_, err := r.db.Exec(`
		INSERT INTO tunnels (id, subdomain, server_url, started_at, ended_at, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, t.ID, t.Subdomain, t.ServerURL, t.StartedAt, t.EndedAt, t.Status)
	if err != nil {
		return fmt.Errorf("insert tunnel: %w", err)
	}
	return nil
}

func (r *SQLiteTunnelRepo) Get(id string) (*Tunnel, error) {
	row := r.db.QueryRow(`
		SELECT id, subdomain, server_url, started_at, ended_at, status
		FROM tunnels WHERE id = ?
	`, id)

	t := &Tunnel{}
	var endedAt sql.NullInt64
	err := row.Scan(&t.ID, &t.Subdomain, &t.ServerURL, &t.StartedAt, &endedAt, &t.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan tunnel: %w", err)
	}
	if endedAt.Valid {
		t.EndedAt = endedAt.Int64
	}
	return t, nil
}

func (r *SQLiteTunnelRepo) UpdateStatus(id string, status string, endedAt int64) error {
	_, err := r.db.Exec(`
		UPDATE tunnels SET status = ?, ended_at = ? WHERE id = ?
	`, status, endedAt, id)
	if err != nil {
		return fmt.Errorf("update tunnel status: %w", err)
	}
	return nil
}

func (r *SQLiteTunnelRepo) ListActive() ([]*Tunnel, error) {
	rows, err := r.db.Query(`
		SELECT id, subdomain, server_url, started_at, ended_at, status
		FROM tunnels WHERE status = 'active' ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query active tunnels: %w", err)
	}
	defer rows.Close()

	var tunnels []*Tunnel
	for rows.Next() {
		t := &Tunnel{}
		var endedAt sql.NullInt64
		err := rows.Scan(&t.ID, &t.Subdomain, &t.ServerURL, &t.StartedAt, &endedAt, &t.Status)
		if err != nil {
			return nil, fmt.Errorf("scan tunnel: %w", err)
		}
		if endedAt.Valid {
			t.EndedAt = endedAt.Int64
		}
		tunnels = append(tunnels, t)
	}
	return tunnels, rows.Err()
}
