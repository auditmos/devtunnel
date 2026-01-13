package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/auditmos/devtunnel/crypto"
	"github.com/auditmos/devtunnel/dashboard"
	"github.com/auditmos/devtunnel/storage"
	"github.com/auditmos/devtunnel/tunnel"
	"github.com/oklog/ulid/v2"
	"github.com/urfave/cli/v2"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	app := NewApp()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func NewApp() *cli.App {
	return &cli.App{
		Name:    "devtunnel",
		Usage:   "expose localhost to the internet",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, buildDate),
		Commands: []*cli.Command{
			serverCommand(),
			clientCommand(),
			replayCommand(),
		},
	}
}

func serverCommand() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "start public gateway server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   8080,
				Usage:   "port to listen on",
			},
			&cli.StringFlag{
				Name:  "domain",
				Usage: "public domain (auto-detected if not set)",
			},
			&cli.BoolFlag{
				Name:  "https",
				Usage: "enable auto-HTTPS with Let's Encrypt (requires domain)",
			},
			&cli.StringFlag{
				Name:  "certs-dir",
				Usage: "certificate cache directory (default: ~/.devtunnel/certs)",
			},
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")
			domain := c.String("domain")
			https := c.Bool("https")
			certsDir := c.String("certs-dir")
			return runServer(port, domain, https, certsDir)
		},
	}
}

func clientCommand() *cli.Command {
	return &cli.Command{
		Name:      "start",
		Usage:     "expose local port to the internet",
		ArgsUsage: "[port]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   "3000",
				Usage:   "local port to expose",
			},
			&cli.StringFlag{
				Name:    "server",
				Aliases: []string{"s"},
				Value:   "localhost:8080",
				Usage:   "upstream server address",
			},
			&cli.BoolFlag{
				Name:  "safe",
				Usage: "scrub sensitive headers before logging",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "output logs to stdout in JSONL format",
			},
			&cli.StringFlag{
				Name:  "dashboard-addr",
				Value: "127.0.0.1:4040",
				Usage: "dashboard listen address",
			},
		},
		Action: func(c *cli.Context) error {
			port := c.String("port")
			if c.NArg() > 0 {
				port = c.Args().First()
			}
			server := c.String("server")
			safe := c.Bool("safe")
			jsonOutput := c.Bool("json")
			dashboardAddr := c.String("dashboard-addr")
			return runClient(port, server, safe, jsonOutput, dashboardAddr)
		},
	}
}

func replayCommand() *cli.Command {
	return &cli.Command{
		Name:      "replay",
		Usage:     "replay a shared request to localhost",
		ArgsUsage: "<url>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   3000,
				Usage:   "local port to replay to",
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return fmt.Errorf("url argument required")
			}
			shareURL := c.Args().First()
			port := c.Int("port")
			return runReplay(shareURL, port)
		},
	}
}

func runServer(port int, domain string, https bool, certsDir string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	dbPath, err := getServerDBPath()
	if err != nil {
		return fmt.Errorf("get server db path: %w", err)
	}

	db, err := storage.OpenServerDB(dbPath)
	if err != nil {
		return fmt.Errorf("open server db: %w", err)
	}
	defer db.Close()

	if err := storage.InitBlobSchema(db); err != nil {
		return fmt.Errorf("init blob schema: %w", err)
	}

	if err := storage.InitRateLimitsSchema(db); err != nil {
		return fmt.Errorf("init rate_limits schema: %w", err)
	}

	if err := storage.SeedRateLimits(db); err != nil {
		return fmt.Errorf("seed rate_limits: %w", err)
	}

	rateLimitRepo := storage.NewSQLiteRateLimitRepo(db)
	limits, err := rateLimitRepo.Get()
	if err != nil {
		return fmt.Errorf("get rate_limits: %w", err)
	}

	blobRepo := &blobRepoAdapter{repo: storage.NewSQLiteBlobRepo(db)}

	httpPort := port
	if https {
		httpPort = 80
	}

	srv := tunnel.NewServer(tunnel.ServerConfig{
		Addr:           fmt.Sprintf(":%d", httpPort),
		Domain:         domain,
		BlobRepo:       blobRepo,
		AutoDomain:     domain == "",
		EnableHTTPS:    https,
		CertsDir:       certsDir,
		Version:        version,
		RequestsPerMin: limits.RequestsPerMin,
		MaxConns:       limits.MaxConcurrentConns,
	})

	srv.SetReadyCallback(func() {
		fmt.Printf("Server ready on %s\n", srv.Addr())
	})

	return srv.Start(ctx)
}

func getServerDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".devtunnel")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.db"), nil
}

func runClient(port, server string, safe, jsonOutput bool, dashboardAddr string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nDisconnecting...")
		cancel()
	}()

	if safe {
		fmt.Println("Safe mode enabled: sensitive headers will be scrubbed")
	}

	dbPath, err := getDBPath()
	if err != nil {
		return fmt.Errorf("get db path: %w", err)
	}

	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	repo := storage.NewSQLiteRequestRepo(db)
	tunnelRepo := storage.NewSQLiteTunnelRepo(db)
	scrubRuleRepo := storage.NewSQLiteScrubRuleRepo(db)

	if err := scrubRuleRepo.Seed(); err != nil {
		return fmt.Errorf("seed scrub rules: %w", err)
	}

	var scrubber *storage.Scrubber
	if safe {
		scrubber, err = storage.NewScrubberWithRepo(scrubRuleRepo)
		if err != nil {
			return fmt.Errorf("init scrubber: %w", err)
		}
	}

	tunnelID := ulid.Make().String()
	dbLogger := storage.NewDBLogger(repo, tunnelID, scrubber)

	var logger tunnel.RequestLogger = dbLogger
	if jsonOutput {
		jsonLogger := storage.NewJSONLogger(os.Stdout, scrubber)
		logger = storage.NewMultiLogger(dbLogger, jsonLogger)
	}

	fmt.Printf("Request logging enabled: %s\n", dbPath)

	overridesDir := filepath.Join(filepath.Dir(dbPath), "overrides")
	dashSrv, err := dashboard.NewServer(dashboard.ServerConfig{
		Addr:          dashboardAddr,
		Repo:          repo,
		ScrubRuleRepo: scrubRuleRepo,
		OverridesDir:  overridesDir,
		LocalAddr:     "localhost:" + port,
		ServerAddr:    server,
	})
	if err != nil {
		return fmt.Errorf("init dashboard: %w", err)
	}

	dashSrv.SetReadyCallback(func() {
		fmt.Printf("Dashboard: http://%s\n", dashSrv.Addr())
	})

	go func() {
		if dashErr := dashSrv.Start(ctx); dashErr != nil {
			fmt.Printf("Dashboard error: %v\n", dashErr)
		}
	}()

	client := tunnel.NewClient(tunnel.ClientConfig{
		ServerAddr: server,
		LocalPort:  port,
	})

	client.SetLogger(logger)

	client.OnConnected(func(publicURL string) {
		fmt.Printf("Forwarding %s -> localhost:%s\n", publicURL, port)
		subdomain := extractSubdomain(publicURL)
		t := &storage.Tunnel{
			ID:        tunnelID,
			Subdomain: subdomain,
			ServerURL: server,
			Status:    "active",
		}
		if err := tunnelRepo.Save(t); err != nil {
			fmt.Printf("save tunnel: %v\n", err)
		}
	})

	client.OnDisconnect(func(err error) {
		fmt.Printf("Disconnected: %v\n", err)
		if updateErr := tunnelRepo.UpdateStatus(tunnelID, "disconnected", time.Now().UnixMilli()); updateErr != nil {
			fmt.Printf("update tunnel status: %v\n", updateErr)
		}
	})

	if err := client.Connect(ctx); err != nil {
		return err
	}
	client.Wait(ctx)
	return nil
}

func getDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".devtunnel")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "devtunnel.db"), nil
}

func extractSubdomain(publicURL string) string {
	parsed, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	parts := strings.Split(host, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

type SharedRequest struct {
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestHeaders  map[string]string `json:"request_headers"`
	RequestBody     string            `json:"request_body"`
	StatusCode      int               `json:"status_code"`
	ResponseHeaders map[string]string `json:"response_headers"`
	ResponseBody    string            `json:"response_body"`
	DurationMs      int64             `json:"duration_ms"`
}

func runReplay(shareURL string, port int) error {
	parsed, err := url.Parse(shareURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}

	keyStr := parsed.Fragment
	if keyStr == "" {
		return fmt.Errorf("missing decryption key in URL hash")
	}

	blobID := strings.TrimPrefix(parsed.Path, "/shared/")
	if blobID == "" {
		return fmt.Errorf("invalid share URL format")
	}

	blobURL := fmt.Sprintf("%s://%s/api/blob/%s", parsed.Scheme, parsed.Host, blobID)
	fmt.Printf("Fetching blob from %s\n", blobURL)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Get(blobURL)
	if err != nil {
		return fmt.Errorf("fetch blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fetch failed: %s", body)
	}

	var blobResp struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&blobResp); err != nil {
		return fmt.Errorf("decode blob response: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(blobResp.Ciphertext)
	if err != nil {
		return fmt.Errorf("decode ciphertext: %w", err)
	}

	key, err := crypto.DecodeKey(keyStr)
	if err != nil {
		return fmt.Errorf("decode key: %w", err)
	}

	plaintext, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	var req SharedRequest
	if err := json.Unmarshal(plaintext, &req); err != nil {
		return fmt.Errorf("unmarshal request: %w", err)
	}

	fmt.Printf("Replaying %s %s to localhost:%d\n", req.Method, req.URL, port)

	localURL := fmt.Sprintf("http://localhost:%d%s", port, req.URL)
	httpReq, err := http.NewRequest(req.Method, localURL, bytes.NewReader([]byte(req.RequestBody)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	for k, v := range req.RequestHeaders {
		httpReq.Header.Set(k, v)
	}

	localResp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("replay request: %w", err)
	}
	defer localResp.Body.Close()

	respBody, err := io.ReadAll(localResp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	fmt.Printf("Response: %d %s\n", localResp.StatusCode, localResp.Status)
	fmt.Printf("Body:\n%s\n", respBody)

	return nil
}

type blobRepoAdapter struct {
	repo *storage.SQLiteBlobRepo
}

func (a *blobRepoAdapter) Save(blob *tunnel.SharedBlob) error {
	return a.repo.Save(&storage.SharedBlob{
		ID:         blob.ID,
		Ciphertext: blob.Ciphertext,
		CreatedAt:  blob.CreatedAt,
		ExpiresAt:  blob.ExpiresAt,
	})
}

func (a *blobRepoAdapter) Get(id string) (*tunnel.SharedBlob, error) {
	b, err := a.repo.Get(id)
	if err != nil || b == nil {
		return nil, err
	}
	return &tunnel.SharedBlob{
		ID:         b.ID,
		Ciphertext: b.Ciphertext,
		CreatedAt:  b.CreatedAt,
		ExpiresAt:  b.ExpiresAt,
	}, nil
}
