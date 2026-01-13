package storage

import (
	"database/sql"
	"fmt"
)

const rateLimitsSchema = `
CREATE TABLE IF NOT EXISTS rate_limits (
    id                   INTEGER PRIMARY KEY CHECK (id = 1),
    requests_per_min     INTEGER NOT NULL,
    max_concurrent_conns INTEGER NOT NULL
);
`

type RateLimits struct {
	RequestsPerMin     int
	MaxConcurrentConns int
}

type RateLimitRepo interface {
	Get() (*RateLimits, error)
}

type SQLiteRateLimitRepo struct {
	db *sql.DB
}

func InitRateLimitsSchema(db *sql.DB) error {
	_, err := db.Exec(rateLimitsSchema)
	if err != nil {
		return fmt.Errorf("init rate_limits schema: %w", err)
	}
	return nil
}

func SeedRateLimits(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO rate_limits (id, requests_per_min, max_concurrent_conns)
		VALUES (1, 60, 5)
	`)
	if err != nil {
		return fmt.Errorf("seed rate_limits: %w", err)
	}
	return nil
}

func NewSQLiteRateLimitRepo(db *sql.DB) *SQLiteRateLimitRepo {
	return &SQLiteRateLimitRepo{db: db}
}

func (r *SQLiteRateLimitRepo) Get() (*RateLimits, error) {
	row := r.db.QueryRow("SELECT requests_per_min, max_concurrent_conns FROM rate_limits WHERE id = 1")
	limits := &RateLimits{}
	err := row.Scan(&limits.RequestsPerMin, &limits.MaxConcurrentConns)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("rate_limits not seeded")
	}
	if err != nil {
		return nil, fmt.Errorf("scan rate_limits: %w", err)
	}
	return limits, nil
}
