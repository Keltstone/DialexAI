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
	Addr      string
	GHSecret  string
	GLToken   string
	WorkDir   string
	JobsToken string
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
	if !isLoopback {
		// Secure by default: non-loopback binds (0.0.0.0, external interfaces) MUST have both webhook authentication and jobs authentication regardless of mode
		if opts.GHSecret == "" && opts.GLToken == "" {
			return nil, fmt.Errorf("security violation: artixd refuses non-loopback bind (%s) without webhook secret (gh-secret or gl-token)", opts.Addr)
		}
		if opts.JobsToken == "" {
			return nil, fmt.Errorf("security violation: artixd refuses non-loopback bind (%s) without jobs-token authentication", opts.Addr)
		}
	} else if policy.IsEnterprise() {
		// Even on loopback, enterprise mode strictly requires webhook secret and jobs token
		if opts.GHSecret == "" && opts.GLToken == "" {
			return nil, fmt.Errorf("enterprise mode requires at least one webhook secret (gh-secret or gl-token)")
		}
		if opts.JobsToken == "" {
			return nil, fmt.Errorf("enterprise mode requires jobs-token authentication")
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
		RequireWebhookAuth: policy.IsEnterprise() || !isLoopback,
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
	_ = fs.Parse(os.Args[1:])

	server, err := BuildDaemonServer(DaemonOptions{
		Addr:      *addr,
		GHSecret:  *ghSecret,
		GLToken:   *glToken,
		WorkDir:   *workDir,
		JobsToken: *jobsToken,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Daemon initialization error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting Artix Daemon on %s...\n", *addr)
	fmt.Printf("Endpoints:\n  - GET  /healthz\n  - GET  /readyz\n  - GET  /metrics\n  - POST /webhook/github\n  - POST /webhook/gitlab\n  - GET  /jobs\n  - GET/POST /tasks\n")

	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "Server fatal error: %v\n", err)
		os.Exit(1)
	}
}
