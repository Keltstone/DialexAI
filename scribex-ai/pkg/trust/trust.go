// Package trust enforces operator-level allowlisting for AI endpoints.
package trust

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// AIHosts returns the set of AI endpoint hosts the OPERATOR (not the repo) trusts, for the given provider.
func AIHosts(provider string, env func(string) string, home string) ([]string, error) {
	var hosts []string
	seen := make(map[string]bool)

	addHost := func(h string) error {
		h = strings.TrimSpace(h)
		if h == "" {
			return nil
		}
		if strings.Contains(h, "://") || strings.Contains(h, "/") || strings.Contains(h, "*") ||
			strings.Contains(h, "?") || strings.Contains(h, "#") || strings.Contains(h, "@") {
			return fmt.Errorf("invalid host entry %q: wildcards, schemes, and paths are not allowed", h)
		}
		if strings.Contains(h, ":") {
			host, port, err := net.SplitHostPort(h)
			if err != nil || host == "" || port == "" {
				return fmt.Errorf("invalid host:port entry %q", h)
			}
			portNum, err := strconv.Atoi(port)
			if err != nil || portNum < 1 || portNum > 65535 {
				return fmt.Errorf("invalid port in host:port entry %q", h)
			}
		}
		hLower := strings.ToLower(h)
		if !seen[hLower] {
			seen[hLower] = true
			hosts = append(hosts, hLower)
		}
		return nil
	}

	// 1. Built-in: anthropic -> api.anthropic.com; openai -> api.openai.com
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		_ = addHost("api.anthropic.com")
	case "openai":
		_ = addHost("api.openai.com")
	}

	// 2. Env SCRIBEX_AI_ALLOWED_HOSTS — comma list of host or host:port
	if env != nil {
		if envVal := env("SCRIBEX_AI_ALLOWED_HOSTS"); envVal != "" {
			for _, part := range strings.Split(envVal, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if err := addHost(part); err != nil {
					return nil, fmt.Errorf("SCRIBEX_AI_ALLOWED_HOSTS: %w", err)
				}
			}
		}
	}

	// 3. User file $XDG_CONFIG_HOME/scribex/trust.yaml else ~/.config/scribex/trust.yaml
	var trustPath string
	if env != nil && env("XDG_CONFIG_HOME") != "" {
		trustPath = filepath.Join(env("XDG_CONFIG_HOME"), "scribex/trust.yaml")
	} else if home != "" {
		trustPath = filepath.Join(home, ".config/scribex/trust.yaml")
	}

	if trustPath != "" {
		fi, err := os.Lstat(trustPath)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("trust file %s is a symlink", trustPath)
			}
			if fi.Mode().Perm()&0o022 != 0 {
				return nil, fmt.Errorf("trust file %s is group or world writable (mode %04o)", trustPath, fi.Mode().Perm())
			}
			if err := checkOwner(fi, trustPath); err != nil {
				return nil, err
			}

			f, err := os.Open(trustPath)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(strings.ToLower(line), "ai_hosts:") {
					val := strings.TrimSpace(line[len("ai_hosts:"):])
					val = strings.Trim(val, "[]")
					for _, part := range strings.Split(val, ",") {
						part = strings.Trim(strings.TrimSpace(part), `"'`)
						if part == "" {
							continue
						}
						if err := addHost(part); err != nil {
							return nil, fmt.Errorf("trust file %s: %w", trustPath, err)
						}
					}
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return hosts, nil
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// CheckAIEndpoint validates baseURL against AIHosts; returns the cleaned URL or an error.
func CheckAIEndpoint(provider, baseURL string, hosts []string, insecureLocal bool) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid ai.base_url: %w", err)
	}
	if u.User != nil {
		return "", fmt.Errorf("ai.base_url must not contain userinfo")
	}
	if u.RawQuery != "" {
		return "", fmt.Errorf("ai.base_url must not contain query")
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("ai.base_url must not contain fragment")
	}
	if u.Host == "" {
		return "", fmt.Errorf("ai.base_url missing host")
	}

	isLoopback := isLoopbackHost(u.Hostname())
	if u.Scheme == "http" {
		if !(isLoopback && insecureLocal) {
			return "", fmt.Errorf("ai.base_url scheme http only allowed for loopback with SCRIBEX_ALLOW_INSECURE_LOCAL=1")
		}
	} else if u.Scheme != "https" {
		return "", fmt.Errorf("ai.base_url scheme must be https, got %q", u.Scheme)
	}

	hostWithPort := strings.ToLower(u.Host)
	hostname := strings.ToLower(u.Hostname())
	trusted := false
	for _, h := range hosts {
		if h == hostWithPort {
			trusted = true
			break
		}
		if u.Port() == "" && h == hostname {
			trusted = true
			break
		}
	}

	if !trusted {
		return "", fmt.Errorf("ai.base_url host %q is not trusted; allow it with SCRIBEX_AI_ALLOWED_HOSTS or ~/.config/scribex/trust.yaml", u.Host)
	}

	cleanedPath := path.Clean(u.Path)
	if cleanedPath == "." || cleanedPath == "/" {
		cleanedPath = ""
	}
	cleanedPath = strings.TrimSuffix(cleanedPath, "/")

	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, cleanedPath), nil
}
