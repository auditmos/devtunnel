package tunnel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPToNipIO(t *testing.T) {
	tests := []struct {
		ip       string
		expected string
	}{
		{"1.2.3.4", "1-2-3-4.nip.io"},
		{"192.168.1.100", "192-168-1-100.nip.io"},
		{"10.0.0.1", "10-0-0-1.nip.io"},
		{"203.0.113.42", "203-0-113-42.nip.io"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := IPToNipIO(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPublicIPWithMockService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("203.0.113.42"))
	}))
	defer srv.Close()

	client := srv.Client()
	ip, err := getPublicIPWithServices([]string{srv.URL}, client)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.42", ip)
}

func TestGetPublicIPWithJSONService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ip":"198.51.100.1"}`))
	}))
	defer srv.Close()

	url := srv.URL + "?format=json"
	ip, err := fetchIP(url, srv.Client())
	require.NoError(t, err)
	assert.Equal(t, "198.51.100.1", ip)
}

func TestGetPublicIPFallback(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	successSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("192.0.2.1"))
	}))
	defer successSrv.Close()

	client := successSrv.Client()
	ip, err := getPublicIPWithServices([]string{failSrv.URL, successSrv.URL}, client)
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1", ip)
}

func TestGetPublicIPAllFail(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	client := failSrv.Client()
	_, err := getPublicIPWithServices([]string{failSrv.URL}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to detect")
}

func TestFetchIPInvalidIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-an-ip"))
	}))
	defer srv.Close()

	_, err := fetchIP(srv.URL, srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IP")
}

func TestFetchIPTrimsWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("  203.0.113.1\n"))
	}))
	defer srv.Close()

	ip, err := fetchIP(srv.URL, srv.Client())
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.1", ip)
}

func TestServerAutoDomainDisabled(t *testing.T) {
	srv := NewServer(ServerConfig{
		Addr:       "127.0.0.1:0",
		AutoDomain: false,
	})
	assert.Empty(t, srv.Domain())
}

func TestServerExplicitDomainOverridesAuto(t *testing.T) {
	srv := NewServer(ServerConfig{
		Addr:       "127.0.0.1:0",
		Domain:     "custom.example.com",
		AutoDomain: true,
	})
	assert.Equal(t, "custom.example.com", srv.Domain())
}
