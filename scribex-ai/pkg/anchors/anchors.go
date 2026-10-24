// Package anchors parses `<!-- @scribex:anchor(file#Symbol) -->` blocks and checks/syncs them against code.
package anchors

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var open = regexp.MustCompile(`^\s*<!--\s*@scribex:anchor\(([^)#]+)#([^)\s]+)\)(?:\s+sig=([0-9a-f]+))?\s*-->\s*$`)

// Lookup returns the current signature of file#sym, ok=false when missing.
type Lookup func(file, sym string) (string, bool)

type Issue struct {
	Line int // 1-based
	Ref  string
	Msg  string
}

func (i Issue) String() string { return fmt.Sprintf(":%d %s: %s", i.Line, i.Ref, i.Msg) }

func Hash(sig string) string {
	h := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(h[:4])
}

// Process walks md; returns issues and, when sync is true, the rewritten text.
// Sync refreshes the sig hash and the ```signature fence; prose is never touched.
func Process(md string, look Lookup, sync bool) ([]Issue, string) {
	in := strings.Split(md, "\n")
	var out []string
	var issues []Issue
	for i := 0; i < len(in); i++ {
		m := open.FindStringSubmatch(in[i])
		if m == nil {
			out = append(out, in[i])
			continue
		}
		ref := m[1] + "#" + m[2]
		sig, ok := look(m[1], m[2])
		if !ok {
			issues = append(issues, Issue{i + 1, ref, "symbol not found (dead anchor)"})
			out = append(out, in[i])
			continue
		}
		want := Hash(sig)
		if m[3] != want {
			issues = append(issues, Issue{i + 1, ref, "stale: signature changed or never synced"})
		}
		if !sync {
			out = append(out, in[i])
			continue
		}
		out = append(out, fmt.Sprintf("<!-- @scribex:anchor(%s) sig=%s -->", ref, want), "```signature", sig, "```")
		// drop an existing signature fence right after the anchor
		if i+1 < len(in) && strings.TrimSpace(in[i+1]) == "```signature" {
			i++
			for i+1 < len(in) && strings.TrimSpace(in[i]) != "```" {
				i++
			}
		}
	}
	return issues, strings.Join(out, "\n")
}
