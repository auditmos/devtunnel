package tunnel

import (
	"crypto/rand"
	"encoding/hex"
)

type HandshakeRequest struct {
	Version   string `json:"version"`
	AuthToken string `json:"auth_token,omitempty"`
	Subdomain string `json:"subdomain,omitempty"`
}

type HandshakeResponse struct {
	Success   bool   `json:"success"`
	Subdomain string `json:"subdomain"`
	PublicURL string `json:"public_url"`
	Error     string `json:"error,omitempty"`
}

type RequestFrame struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

type ResponseFrame struct {
	ID         string            `json:"id"`
	StatusCode int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

func generateSubdomain() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
