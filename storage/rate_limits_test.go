package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRateLimitsSchema(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitRateLimitsSchema(db)
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM rate_limits").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestSeedRateLimits(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitRateLimitsSchema(db)
	require.NoError(t, err)

	err = SeedRateLimits(db)
	require.NoError(t, err)

	repo := NewSQLiteRateLimitRepo(db)
	limits, err := repo.Get()
	require.NoError(t, err)
	assert.Equal(t, 60, limits.RequestsPerMin)
	assert.Equal(t, 5, limits.MaxConcurrentConns)
}

func TestSeedRateLimitsIdempotent(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitRateLimitsSchema(db)
	require.NoError(t, err)

	err = SeedRateLimits(db)
	require.NoError(t, err)
	err = SeedRateLimits(db)
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM rate_limits").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRateLimitRepoGetNotSeeded(t *testing.T) {
	db, err := OpenMemoryDB()
	require.NoError(t, err)
	defer db.Close()

	err = InitRateLimitsSchema(db)
	require.NoError(t, err)

	repo := NewSQLiteRateLimitRepo(db)
	_, err = repo.Get()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not seeded")
}
