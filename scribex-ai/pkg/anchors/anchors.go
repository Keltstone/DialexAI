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
	Line  int // 1-based
	Ref   string
	Msg   string
	Stale bool // fixable by sync; false = dead anchor
}

func (i Issue) String() string { return fmt.Sprintf(":%d %s: %s", i.Line, i.Ref, i.Msg) }

func Hash(sig string) string {
	h := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(h[:4])
}

// Prose rewrites an anchor's prose for a changed signature. Only called for anchors that were
// synced before (sig= present) and whose signature changed, so first-time syncs never spend tokens.
type Prose func(ref, sig, old string) (string, error)

const endMark = "<!-- @scribex:end -->"

// Process walks md; returns issues and, when sync is true, the rewritten text.
// Sync refreshes the sig hash and the ```signature fence; prose is never touched.
func Process(md string, look Lookup, sync bool, prose Prose) ([]Issue, string) {
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
			issues = append(issues, Issue{i + 1, ref, "symbol not found (dead anchor)", false})
			out = append(out, in[i])
			continue
		}
		want := Hash(sig)
		if m[3] != want {
			issues = append(issues, Issue{i + 1, ref, "stale: signature changed or never synced", true})
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
		if prose != nil && m[3] != "" && m[3] != want {
			if j := endAt(in, i+1); j >= 0 {
				nw, err := prose(ref, sig, strings.Join(in[i+1:j], "\n"))
				if err != nil {
					issues = append(issues, Issue{Line: i + 1, Ref: ref, Msg: "ai rewrite failed: " + err.Error()})
				} else {
					out = append(out, strings.Split(strings.Trim(nw, "\n"), "\n")...)
					i = j - 1
				}
			}
		}
	}
	return issues, strings.Join(out, "\n")
}

// Count returns the number of anchors in md.
func Count(md string) (n int) {
	for _, l := range strings.Split(md, "\n") {
		if open.MatchString(l) {
			n++
		}
	}
	return
}

func endAt(lines []string, from int) int {
	for j := from; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == endMark {
			return j
		}
		if open.MatchString(lines[j]) {
			return -1
		}
	}
	return -1
}
