package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrubRuleRepo_Seed(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)
	err = repo.Seed()
	require.NoError(t, err)

	rules, err := repo.GetAll()
	require.NoError(t, err)
	assert.Len(t, rules, len(defaultScrubPatterns))

	patterns := make(map[string]bool)
	for _, rule := range rules {
		patterns[rule.Pattern] = true
	}
	for _, expected := range defaultScrubPatterns {
		assert.True(t, patterns[expected], "missing pattern: %s", expected)
	}
}

func TestScrubRuleRepo_SeedIdempotent(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	require.NoError(t, repo.Seed())
	require.NoError(t, repo.Seed())

	rules, err := repo.GetAll()
	require.NoError(t, err)
	assert.Len(t, rules, len(defaultScrubPatterns))
}

func TestScrubRuleRepo_Create(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	rule, err := repo.Create("x-custom-secret")
	require.NoError(t, err)
	assert.NotEmpty(t, rule.ID)
	assert.Equal(t, "x-custom-secret", rule.Pattern)
	assert.NotZero(t, rule.CreatedAt)

	rules, err := repo.GetAll()
	require.NoError(t, err)
	assert.Len(t, rules, 1)
	assert.Equal(t, rule.ID, rules[0].ID)
}

func TestScrubRuleRepo_CreateEmptyPattern(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	_, err = repo.Create("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestScrubRuleRepo_CreateDuplicate(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	_, err = repo.Create("x-secret")
	require.NoError(t, err)

	_, err = repo.Create("x-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE constraint")
}

func TestScrubRuleRepo_Delete(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	rule, err := repo.Create("x-secret")
	require.NoError(t, err)

	err = repo.Delete(rule.ID)
	require.NoError(t, err)

	rules, err := repo.GetAll()
	require.NoError(t, err)
	assert.Len(t, rules, 0)
}

func TestScrubRuleRepo_DeleteNotFound(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	err = repo.Delete("nonexistent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestScrubRuleRepo_GetAllEmpty(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)

	rules, err := repo.GetAll()
	require.NoError(t, err)
	assert.Len(t, rules, 0)
}

func TestScrubber_ReloadsAfterCreate(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)
	require.NoError(t, repo.Seed())

	scrubber, err := NewScrubberWithRepo(repo)
	require.NoError(t, err)

	headers := map[string]string{"x-custom-secret": "value123"}
	result := scrubber.ScrubHeaders(headers)
	assert.Equal(t, "value123", result["x-custom-secret"])

	_, err = repo.Create("x-custom-secret")
	require.NoError(t, err)

	require.NoError(t, scrubber.Reload())

	result = scrubber.ScrubHeaders(headers)
	assert.Equal(t, "***", result["x-custom-secret"])
}

func TestScrubber_ReloadsAfterDelete(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLiteScrubRuleRepo(db)
	require.NoError(t, repo.Seed())

	scrubber, err := NewScrubberWithRepo(repo)
	require.NoError(t, err)

	headers := map[string]string{"authorization": "Bearer token"}
	result := scrubber.ScrubHeaders(headers)
	assert.Equal(t, "***", result["authorization"])

	rules, err := repo.GetAll()
	require.NoError(t, err)
	var authRuleID string
	for _, r := range rules {
		if r.Pattern == "authorization" {
			authRuleID = r.ID
			break
		}
	}
	require.NotEmpty(t, authRuleID)

	require.NoError(t, repo.Delete(authRuleID))
	require.NoError(t, scrubber.Reload())

	result = scrubber.ScrubHeaders(headers)
	assert.Equal(t, "Bearer token", result["authorization"])
}
