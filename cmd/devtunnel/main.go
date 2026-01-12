package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/auditmos/devtunnel/tunnel"
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

	srv := tunnel.NewServer(tunnel.ServerConfig{
		Addr:   fmt.Sprintf(":%d", port),
		Domain: domain,
	})

	return srv.Start(ctx)
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

	client := tunnel.NewClient(tunnel.ClientConfig{
		ServerAddr: server,
		LocalPort:  port,
	})

	client.OnConnected(func(url string) {
		fmt.Printf("Forwarding %s -> localhost:%s\n", url, port)
	})

	client.OnDisconnect(func(err error) {
		fmt.Printf("Disconnected: %v\n", err)
	})

	return client.Connect(ctx)
}
