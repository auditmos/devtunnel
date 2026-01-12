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

var version = "dev"

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
		Version: version,
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
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")
			domain := c.String("domain")
			return runServer(port, domain)
		},
	}
}

func clientCommand() *cli.Command {
	return &cli.Command{
		Name:      "start",
		Usage:     "expose local port to the internet",
		ArgsUsage: "<port>",
		Flags: []cli.Flag{
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
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return fmt.Errorf("port argument required")
			}
			port := c.Args().First()
			server := c.String("server")
			safe := c.Bool("safe")
			return runClient(port, server, safe)
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

func runServer(port int, domain string) error {
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

	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open server db: %w", err)
	}
	defer db.Close()

	if err := storage.InitBlobSchema(db); err != nil {
		return fmt.Errorf("init blob schema: %w", err)
	}

	blobRepo := &blobRepoAdapter{repo: storage.NewSQLiteBlobRepo(db)}

	srv := tunnel.NewServer(tunnel.ServerConfig{
		Addr:     fmt.Sprintf(":%d", port),
		Domain:   domain,
		BlobRepo: blobRepo,
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

func runClient(port, server string, safe bool) error {
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
	tunnelID := ulid.Make().String()
	logger := storage.NewDBLogger(repo, tunnelID, safe)

	fmt.Printf("Request logging enabled: %s\n", dbPath)

	overridesDir := filepath.Join(filepath.Dir(dbPath), "overrides")
	dashSrv, err := dashboard.NewServer(dashboard.ServerConfig{
		Addr:         ":4040",
		Repo:         repo,
		OverridesDir: overridesDir,
		LocalAddr:    "localhost:" + port,
		ServerAddr:   server,
	})
	if err != nil {
		return fmt.Errorf("init dashboard: %w", err)
	}

	go func() {
		fmt.Println("Dashboard: http://localhost:4040")
		if dashErr := dashSrv.Start(ctx); dashErr != nil {
			fmt.Printf("Dashboard error: %v\n", dashErr)
		}
	}()

	client := tunnel.NewClient(tunnel.ClientConfig{
		ServerAddr: server,
		LocalPort:  port,
	})

	client.SetLogger(logger)

	client.OnConnected(func(url string) {
		fmt.Printf("Forwarding %s -> localhost:%s\n", url, port)
	})

	client.OnDisconnect(func(err error) {
		fmt.Printf("Disconnected: %v\n", err)
	})

	return client.Connect(ctx)
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
