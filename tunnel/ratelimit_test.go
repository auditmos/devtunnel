package tunnel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiterAllowRequest(t *testing.T) {
	rl := NewRateLimiter(3, 5)

	ok, _ := rl.AllowRequest("sub1")
	assert.True(t, ok)

	ok, _ = rl.AllowRequest("sub1")
	assert.True(t, ok)

	ok, _ = rl.AllowRequest("sub1")
	assert.True(t, ok)

	ok, retryAfter := rl.AllowRequest("sub1")
	assert.False(t, ok)
	assert.Greater(t, retryAfter, 0)
}

func TestRateLimiterIsolatesSubdomains(t *testing.T) {
	rl := NewRateLimiter(2, 5)

	ok, _ := rl.AllowRequest("sub1")
	assert.True(t, ok)
	ok, _ = rl.AllowRequest("sub1")
	assert.True(t, ok)
	ok, _ = rl.AllowRequest("sub1")
	assert.False(t, ok)

	ok, _ = rl.AllowRequest("sub2")
	assert.True(t, ok)
	ok, _ = rl.AllowRequest("sub2")
	assert.True(t, ok)
}

func TestRateLimiterConnectionLimit(t *testing.T) {
	rl := NewRateLimiter(60, 2)

	assert.True(t, rl.AcquireConnection("sub1"))
	assert.True(t, rl.AcquireConnection("sub1"))
	assert.False(t, rl.AcquireConnection("sub1"))

	rl.ReleaseConnection("sub1")
	assert.True(t, rl.AcquireConnection("sub1"))
}

func TestRateLimiterConnectionIsolatesSubdomains(t *testing.T) {
	rl := NewRateLimiter(60, 1)

	assert.True(t, rl.AcquireConnection("sub1"))
	assert.False(t, rl.AcquireConnection("sub1"))

	assert.True(t, rl.AcquireConnection("sub2"))
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(60, 5)

	rl.AllowRequest("sub1")
	rl.AcquireConnection("sub1")

	rl.CleanupSubdomain("sub1")

	rl.mu.Lock()
	_, hasWindow := rl.windows["sub1"]
	_, hasConn := rl.connCounts["sub1"]
	rl.mu.Unlock()

	assert.False(t, hasWindow)
	assert.False(t, hasConn)
}

func TestRateLimiterGetLimits(t *testing.T) {
	rl := NewRateLimiter(100, 10)
	rpm, mc := rl.GetLimits()
	assert.Equal(t, 100, rpm)
	assert.Equal(t, 10, mc)
}

func TestWriteRateLimitExceeded(t *testing.T) {
	w := httptest.NewRecorder()
	WriteRateLimitExceeded(w, 30)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "30", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), "rate limit exceeded")
}

func TestWriteConnectionLimitExceeded(t *testing.T) {
	w := httptest.NewRecorder()
	WriteConnectionLimitExceeded(w)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "connection limit exceeded")
}
