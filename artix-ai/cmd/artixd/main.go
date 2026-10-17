package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"artix/pkg/forge"
	"artix/pkg/persona"
	"artix/pkg/policy"
)

// DaemonOptions configures the Artix daemon server.
type DaemonOptions struct {
	Addr        string
	GHSecret    string
	GLToken     string
	WorkDir     string
	JobsToken   string
	InsecureDev bool
}

func isLoopbackAddr(addr string) bool {
	if addr == "" {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// BuildDaemonServer creates and configures a WebhookServer, validating security constraints.
// It refuses non-loopback binds without authentication regardless of enterprise mode.
func BuildDaemonServer(opts DaemonOptions) (*forge.WebhookServer, error) {
	isLoopback := isLoopbackAddr(opts.Addr)
	if !opts.InsecureDev {
		if opts.GHSecret == "" && opts.GLToken == "" {
			return nil, fmt.Errorf("security violation: artixd requires webhook secret (gh-secret or gl-token) unless --insecure-dev on loopback")
		}
		if opts.JobsToken == "" {
			return nil, fmt.Errorf("security violation: artixd requires jobs-token authentication unless --insecure-dev on loopback")
		}
	} else {
		if !isLoopback {
			return nil, fmt.Errorf("security violation: --insecure-dev is strictly prohibited on non-loopback address (%s)", opts.Addr)
		}
	}

	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Join(os.TempDir(), "artix-worker")
	}

	cwd, _ := os.Getwd()
	registry := persona.NewRegistry(cwd)
	worker := forge.NewRemoteWorker(opts.WorkDir, registry)

	cfg := forge.WebhookServerConfig{
		ListenAddr:         opts.Addr,
		GitHubSecret:       opts.GHSecret,
		GitLabToken:        opts.GLToken,
		JobsAuthToken:      opts.JobsToken,
		RequireWebhookAuth: !opts.InsecureDev,
		RequireAuth:        !opts.InsecureDev,
		InsecureDev:        opts.InsecureDev,
		Worker:             worker,
	}

	return forge.NewWebhookServer(cfg), nil
}

func main() {
	fs := flag.NewFlagSet("artixd", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "Server listen address")
	ghSecret := fs.String("gh-secret", os.Getenv("GITHUB_WEBHOOK_SECRET"), "GitHub Webhook HMAC SHA256 secret")
	glToken := fs.String("gl-token", os.Getenv("GITLAB_WEBHOOK_TOKEN"), "GitLab Webhook token")
	workDir := fs.String("work-dir", filepath.Join(os.TempDir(), "artix-worker"), "Directory for ephemeral git clones")
	jobsToken := fs.String("jobs-token", os.Getenv("ARTIX_JOBS_TOKEN"), "Bearer token required to query /jobs")
	insecureDev := fs.Bool("insecure-dev", false, "Allow unauthenticated dev access on loopback only")
	_ = fs.Parse(os.Args[1:])

	server, err := BuildDaemonServer(DaemonOptions{
		Addr:        *addr,
		GHSecret:    *ghSecret,
		GLToken:     *glToken,
		WorkDir:     *workDir,
		JobsToken:   *jobsToken,
		InsecureDev: *insecureDev,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Daemon initialization error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting Artix Daemon on %s...\n", *addr)
	fmt.Printf("Endpoints:\n  - GET  /healthz\n  - GET  /readyz\n  - GET  /metrics\n  - POST /webhook/github\n  - POST /webhook/gitlab\n  - GET  /jobs\n  - GET/POST /tasks\n")

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		fmt.Println("\nShutting down Artix Daemon gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		server.Drain(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Server fatal error: %v\n", err)
		os.Exit(1)
	}
}
