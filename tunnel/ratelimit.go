package tunnel

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type RateLimiter struct {
	mu             sync.Mutex
	requestsPerMin int
	maxConns       int
	windows        map[string]*slidingWindow
	connCounts     map[string]int
}

type slidingWindow struct {
	timestamps []int64
}

func NewRateLimiter(requestsPerMin, maxConns int) *RateLimiter {
	return &RateLimiter{
		requestsPerMin: requestsPerMin,
		maxConns:       maxConns,
		windows:        make(map[string]*slidingWindow),
		connCounts:     make(map[string]int),
	}
}

func (rl *RateLimiter) AllowRequest(subdomain string) (bool, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UnixMilli()
	windowStart := now - 60000 // 1 minute window

	window, ok := rl.windows[subdomain]
	if !ok {
		window = &slidingWindow{}
		rl.windows[subdomain] = window
	}

	// prune old timestamps
	var valid []int64
	for _, ts := range window.timestamps {
		if ts > windowStart {
			valid = append(valid, ts)
		}
	}
	window.timestamps = valid

	if len(window.timestamps) >= rl.requestsPerMin {
		// calculate retry-after (oldest timestamp + 60s - now)
		oldest := window.timestamps[0]
		retryAfter := int((oldest + 60000 - now) / 1000)
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}

	window.timestamps = append(window.timestamps, now)
	return true, 0
}

func (rl *RateLimiter) AcquireConnection(subdomain string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	count := rl.connCounts[subdomain]
	if count >= rl.maxConns {
		return false
	}
	rl.connCounts[subdomain] = count + 1
	return true
}

func (rl *RateLimiter) ReleaseConnection(subdomain string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	count := rl.connCounts[subdomain]
	if count > 0 {
		rl.connCounts[subdomain] = count - 1
	}
	if rl.connCounts[subdomain] == 0 {
		delete(rl.connCounts, subdomain)
	}
}

func (rl *RateLimiter) CleanupSubdomain(subdomain string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.windows, subdomain)
	delete(rl.connCounts, subdomain)
}

func (rl *RateLimiter) GetLimits() (requestsPerMin, maxConns int) {
	return rl.requestsPerMin, rl.maxConns
}

func WriteRateLimitExceeded(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.WriteHeader(http.StatusTooManyRequests)
	w.Write([]byte(`{"error":"rate limit exceeded"}`))
}

func WriteConnectionLimitExceeded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`{"error":"connection limit exceeded"}`))
}
