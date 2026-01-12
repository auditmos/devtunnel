package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

const blobSchema = `
CREATE TABLE IF NOT EXISTS shared_blobs (
    id         TEXT PRIMARY KEY,
    ciphertext BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_blobs_expires ON shared_blobs(expires_at);
`

type SharedBlob struct {
	ID         string
	Ciphertext []byte
	CreatedAt  int64
	ExpiresAt  int64
}

type BlobRepo interface {
	Save(blob *SharedBlob) error
	Get(id string) (*SharedBlob, error)
	Delete(id string) error
	Prune() (int64, error)
}

type SQLiteBlobRepo struct {
	db *sql.DB
}

func InitBlobSchema(db *sql.DB) error {
	_, err := db.Exec(blobSchema)
	if err != nil {
		return fmt.Errorf("init blob schema: %w", err)
	}
	return nil
}

func NewSQLiteBlobRepo(db *sql.DB) *SQLiteBlobRepo {
	return &SQLiteBlobRepo{db: db}
}

func (r *SQLiteBlobRepo) Save(blob *SharedBlob) error {
	if blob.ID == "" {
		blob.ID = ulid.Make().String()
	}
	if blob.CreatedAt == 0 {
		blob.CreatedAt = time.Now().UnixMilli()
	}
	if blob.ExpiresAt == 0 {
		blob.ExpiresAt = time.Now().Add(24 * time.Hour).UnixMilli()
	}

	_, err := r.db.Exec(`
		INSERT INTO shared_blobs (id, ciphertext, created_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, blob.ID, blob.Ciphertext, blob.CreatedAt, blob.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert blob: %w", err)
	}
	return nil
}

func (r *SQLiteBlobRepo) Get(id string) (*SharedBlob, error) {
	row := r.db.QueryRow(`
		SELECT id, ciphertext, created_at, expires_at
		FROM shared_blobs WHERE id = ? AND expires_at > ?
	`, id, time.Now().UnixMilli())

	blob := &SharedBlob{}
	err := row.Scan(&blob.ID, &blob.Ciphertext, &blob.CreatedAt, &blob.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan blob: %w", err)
	}
	return blob, nil
}

func (r *SQLiteBlobRepo) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM shared_blobs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

func (r *SQLiteBlobRepo) Prune() (int64, error) {
	res, err := r.db.Exec("DELETE FROM shared_blobs WHERE expires_at < ?", time.Now().UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("prune blobs: %w", err)
	}
	return res.RowsAffected()
}
