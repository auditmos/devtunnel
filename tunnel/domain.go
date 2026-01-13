package tunnel

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type PublicIPFetcher func() (string, error)

var defaultIPServices = []string{
	"https://api.ipify.org?format=json",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
}

func GetPublicIP() (string, error) {
	return getPublicIPWithServices(defaultIPServices, &http.Client{Timeout: 5 * time.Second})
}

func getPublicIPWithServices(services []string, client *http.Client) (string, error) {
	for _, svc := range services {
		ip, err := fetchIP(svc, client)
		if err == nil && ip != "" {
			return ip, nil
		}
	}
	return "", fmt.Errorf("failed to detect public IP from all services")
}

func fetchIP(url string, client *http.Client) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))

	if strings.Contains(url, "format=json") || strings.HasPrefix(ip, "{") {
		var result struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(body, &result); err == nil && result.IP != "" {
			ip = result.IP
		}
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP: %s", ip)
	}

	return ip, nil
}

func IPToNipIO(ip string) string {
	replaced := strings.ReplaceAll(ip, ".", "-")
	return fmt.Sprintf("%s.nip.io", replaced)
}

func AutoDetectDomain() (string, error) {
	ip, err := GetPublicIP()
	if err != nil {
		return "", fmt.Errorf("detect public IP: %w", err)
	}
	return IPToNipIO(ip), nil
}
