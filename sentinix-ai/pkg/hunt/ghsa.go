package hunt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ghsaRe = regexp.MustCompile(`^GHSA(-[2-9cfghjmpqrvwx]{4}){3}$`)

// FetchGHSA downloads a public GitHub Security Advisory and returns its text for Engine.Variant. Only the
// advisory id leaves the machine. base is the API root ("" = https://api.github.com).
func FetchGHSA(ctx context.Context, id, base string) (string, error) {
	if !ghsaRe.MatchString(id) {
		return "", fmt.Errorf("invalid GHSA id %q", id)
	}
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/advisories/"+id, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("advisory %s: %s", id, resp.Status)
	}
	var a struct {
		Summary, Description string
		CWEs                 []struct {
			CWEID string `json:"cwe_id"`
		} `json:"cwes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&a); err != nil {
		return "", err
	}
	var cs []string
	for _, c := range a.CWEs {
		cs = append(cs, c.CWEID)
	}
	return a.Summary + "\n" + strings.Join(cs, ", ") + "\n" + a.Description, nil
}
