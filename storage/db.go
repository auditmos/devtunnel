package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const clientSchema = `
CREATE TABLE IF NOT EXISTS tunnels (
    id          TEXT PRIMARY KEY,
    subdomain   TEXT NOT NULL,
    server_url  TEXT NOT NULL,
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    status      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tunnels_started ON tunnels(started_at DESC);

CREATE TABLE IF NOT EXISTS requests (
    id              TEXT PRIMARY KEY,
    tunnel_id       TEXT NOT NULL,
    timestamp       INTEGER NOT NULL,
    method          TEXT NOT NULL,
    url             TEXT NOT NULL,
    request_headers TEXT NOT NULL,
    request_body    BLOB,
    status_code     INTEGER,
    response_headers TEXT,
    response_body   BLOB,
    duration_ms     INTEGER,
    created_at      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_requests_tunnel ON requests(tunnel_id);
CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON requests(timestamp DESC);

CREATE TABLE IF NOT EXISTS scrub_rules (
    id         TEXT PRIMARY KEY,
    pattern    TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scrub_rules_pattern ON scrub_rules(pattern);
`

// OpenDB opens client database with tunnels and requests tables
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec(clientSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

// OpenServerDB opens server database (blobs only, no tunnels/requests)
func OpenServerDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}

func OpenMemoryDB() (*sql.DB, error) {
	return OpenDB(":memory:")
}
