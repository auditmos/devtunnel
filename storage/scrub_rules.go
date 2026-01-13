package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

var defaultScrubPatterns = []string{
	"authorization",
	"x-api-key",
	"api-key",
	"api_key",
	"apikey",
	"x-auth-token",
	"x-access-token",
	"cookie",
	"set-cookie",
	"x-csrf-token",
	"x-xsrf-token",
}

type ScrubRule struct {
	ID        string
	Pattern   string
	CreatedAt int64
}

type ScrubRuleRepo interface {
	GetAll() ([]*ScrubRule, error)
	Create(pattern string) (*ScrubRule, error)
	Delete(id string) error
	Seed() error
}

type SQLiteScrubRuleRepo struct {
	db *sql.DB
}

func NewSQLiteScrubRuleRepo(db *sql.DB) *SQLiteScrubRuleRepo {
	return &SQLiteScrubRuleRepo{db: db}
}

func (r *SQLiteScrubRuleRepo) GetAll() ([]*ScrubRule, error) {
	rows, err := r.db.Query("SELECT id, pattern, created_at FROM scrub_rules ORDER BY created_at ASC")
	if err != nil {
		return nil, fmt.Errorf("query scrub_rules: %w", err)
	}
	defer rows.Close()

	var rules []*ScrubRule
	for rows.Next() {
		rule := &ScrubRule{}
		if err := rows.Scan(&rule.ID, &rule.Pattern, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan scrub_rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *SQLiteScrubRuleRepo) Create(pattern string) (*ScrubRule, error) {
	if pattern == "" {
		return nil, fmt.Errorf("pattern cannot be empty")
	}

	rule := &ScrubRule{
		ID:        ulid.Make().String(),
		Pattern:   pattern,
		CreatedAt: time.Now().UnixMilli(),
	}

	_, err := r.db.Exec(
		"INSERT INTO scrub_rules (id, pattern, created_at) VALUES (?, ?, ?)",
		rule.ID, rule.Pattern, rule.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert scrub_rule: %w", err)
	}
	return rule, nil
}

func (r *SQLiteScrubRuleRepo) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM scrub_rules WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete scrub_rule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("scrub rule not found")
	}
	return nil
}

func (r *SQLiteScrubRuleRepo) Seed() error {
	for _, pattern := range defaultScrubPatterns {
		_, err := r.db.Exec(
			"INSERT OR IGNORE INTO scrub_rules (id, pattern, created_at) VALUES (?, ?, ?)",
			ulid.Make().String(), pattern, time.Now().UnixMilli(),
		)
		if err != nil {
			return fmt.Errorf("seed scrub_rule %s: %w", pattern, err)
		}
	}
	return nil
}
