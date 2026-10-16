package advisories

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sentinix/pkg/engines"
)

// Client talks to an OSV API (api.osv.dev or a self-hosted mirror). Only package names and versions leave the network.
type Client struct {
	URL  string
	HTTP *http.Client
}

func (c Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, rd)
	if err != nil {
		return err
	}
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("osv %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type vuln struct {
	ID       string
	Summary  string
	Aliases  []string
	Affected []struct {
		Package struct{ Name, Ecosystem string }
		Ranges  []struct {
			Events []struct{ Fixed string }
		}
	}
	DatabaseSpecific struct{ Severity string } `json:"database_specific"`
}

func severity(s string) engines.Severity {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return engines.Critical
	case "HIGH":
		return engines.High
	case "LOW":
		return engines.Low
	}
	return engines.Medium
}

// Match returns one finding per (component, advisory). A failed lookup is an error, never "no vulnerabilities".
func (c Client) Match(ctx context.Context, comps []Component) ([]engines.Finding, error) {
	type q struct {
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
		Version string `json:"version"`
	}
	hits := map[int][]string{}
	for start := 0; start < len(comps); start += 500 {
		end := min(start+500, len(comps))
		var qs struct {
			Queries []q `json:"queries"`
		}
		for _, k := range comps[start:end] {
			var x q
			x.Package.Name, x.Package.Ecosystem, x.Version = k.Name, k.Ecosystem, k.Version
			qs.Queries = append(qs.Queries, x)
		}
		var res struct {
			Results []struct{ Vulns []struct{ ID string } }
		}
		if err := c.do(ctx, "POST", "/v1/querybatch", qs, &res); err != nil {
			return nil, err
		}
		if len(res.Results) != end-start {
			return nil, fmt.Errorf("osv: %d results for %d queries", len(res.Results), end-start)
		}
		for i, r := range res.Results {
			for _, v := range r.Vulns {
				hits[start+i] = append(hits[start+i], v.ID)
			}
		}
	}
	details := map[string]*vuln{}
	var out []engines.Finding
	for i := 0; i < len(comps); i++ {
		for _, id := range hits[i] {
			v, ok := details[id]
			if !ok {
				v = &vuln{}
				if err := c.do(ctx, "GET", "/v1/vulns/"+id, nil, v); err != nil {
					return nil, err
				}
				details[id] = v
			}
			k := comps[i]
			fix := ""
			for _, a := range v.Affected {
				if a.Package.Name == k.Name && a.Package.Ecosystem == k.Ecosystem {
					for _, r := range a.Ranges {
						for _, e := range r.Events {
							if e.Fixed != "" && fix == "" {
								fix = "upgrade to " + e.Fixed + " or later"
							}
						}
					}
				}
			}
			msg := v.Summary
			if msg == "" {
				msg = "known vulnerability"
			}
			out = append(out, engines.Finding{Rule: "advisory/" + id, Engine: "advisories", File: k.File, Line: 1,
				Severity: severity(v.DatabaseSpecific.Severity), Message: k.Name + ": " + msg,
				Evidence: k.Name + "@" + k.Version, Fix: fix})
		}
	}
	return out, nil
}

// Engine runs the SBOM match as an ordinary engine. Off unless a Client URL is configured.
type Engine struct{ Client Client }

func (Engine) Name() string { return "advisories" }

func (e Engine) Scan(ctx context.Context, root string) ([]engines.Finding, error) {
	return e.Client.Match(ctx, Components(root))
}
