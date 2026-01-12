package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubber_ScrubsAuthorizationHeader(t *testing.T) {
	s := NewScrubber()
	headers := map[string]string{
		"Authorization": "Bearer secret-token-123",
		"Content-Type":  "application/json",
	}

	result := s.ScrubHeaders(headers)

	assert.Equal(t, "***", result["Authorization"])
	assert.Equal(t, "application/json", result["Content-Type"])
}

func TestScrubber_ScrubsAPIKeyVariations(t *testing.T) {
	s := NewScrubber()
	headers := map[string]string{
		"X-Api-Key":    "key1",
		"api-key":      "key2",
		"api_key":      "key3",
		"apikey":       "key4",
		"X-Auth-Token": "token1",
	}

	result := s.ScrubHeaders(headers)

	for k := range headers {
		assert.Equal(t, "***", result[k], "header %s should be scrubbed", k)
	}
}

func TestScrubber_CaseInsensitive(t *testing.T) {
	s := NewScrubber()
	headers := map[string]string{
		"AUTHORIZATION": "Bearer token",
		"authorization": "Bearer token",
		"Authorization": "Bearer token",
	}

	result := s.ScrubHeaders(headers)

	for k := range headers {
		assert.Equal(t, "***", result[k])
	}
}

func TestScrubber_PreservesNonSensitiveHeaders(t *testing.T) {
	s := NewScrubber()
	headers := map[string]string{
		"Content-Type":   "application/json",
		"Accept":         "text/html",
		"X-Custom":       "value",
		"Content-Length": "123",
	}

	result := s.ScrubHeaders(headers)

	assert.Equal(t, headers, result)
}

func TestScrubber_HandlesNilHeaders(t *testing.T) {
	s := NewScrubber()
	result := s.ScrubHeaders(nil)
	assert.Nil(t, result)
}

func TestScrubber_HandlesEmptyHeaders(t *testing.T) {
	s := NewScrubber()
	result := s.ScrubHeaders(map[string]string{})
	assert.Empty(t, result)
}

func TestScrubber_ScrubsCookies(t *testing.T) {
	s := NewScrubber()
	headers := map[string]string{
		"Cookie":     "session=abc123",
		"Set-Cookie": "session=abc123; Path=/",
	}

	result := s.ScrubHeaders(headers)

	assert.Equal(t, "***", result["Cookie"])
	assert.Equal(t, "***", result["Set-Cookie"])
}

func TestScrubber_ScrubsCSRFTokens(t *testing.T) {
	s := NewScrubber()
	headers := map[string]string{
		"X-CSRF-Token": "token123",
		"X-XSRF-Token": "token456",
	}

	result := s.ScrubHeaders(headers)

	assert.Equal(t, "***", result["X-CSRF-Token"])
	assert.Equal(t, "***", result["X-XSRF-Token"])
}
