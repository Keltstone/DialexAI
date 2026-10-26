// Package anchors parses `<!-- @scribex:anchor(file#Symbol) -->` blocks and checks/syncs them against code.
package anchors

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"scribex/pkg/prose"
)

var open = regexp.MustCompile(`^\s*<!--\s*@scribex:anchor\(([^)#]+)#([^)\s]+)\)(?:\s+sig=([0-9a-f]+))?\s*-->\s*$`)

// Lookup returns the current signature of file#sym, ok=false when missing.
type Lookup func(file, sym string) (sig string, ok bool, err error)

const (
	CodeAIRejected = "ai-rejected"
	CodeAIFailed   = "ai-failed"
	CodeAISkipped  = "ai-skipped"
)

type Issue struct {
	Line  int // 1-based
	Ref   string
	Msg   string
	Stale bool   // fixable by sync; false = dead anchor
	Code  string // ai-rejected | ai-failed | ai-skipped
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
func Process(md string, look Lookup, sync bool, proseFn Prose) ([]Issue, string) {
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
		sig, ok, err := look(m[1], m[2])
		if err != nil {
			issues = append(issues, Issue{
				Line:  i + 1,
				Ref:   ref,
				Msg:   "cannot analyse " + m[1] + ": " + err.Error(),
				Stale: false,
			})
			out = append(out, in[i])
			continue
		}
		if !ok {
			issues = append(issues, Issue{
				Line:  i + 1,
				Ref:   ref,
				Msg:   "symbol not found (dead anchor)",
				Stale: false,
			})
			out = append(out, in[i])
			continue
		}
		want := Hash(sig)
		oldSig := m[3]
		isStale := oldSig != want

		if !sync {
			if isStale {
				issues = append(issues, Issue{
					Line:  i + 1,
					Ref:   ref,
					Msg:   "stale: signature changed or never synced",
					Stale: true,
				})
			}
			out = append(out, in[i])
			continue
		}

		// When sync is true:
		// If not stale: refresh signature fence if present to remain idempotent
		if !isStale {
			out = append(out, fmt.Sprintf("<!-- @scribex:anchor(%s) sig=%s -->", ref, want), "```signature", sig, "```")
			if i+1 < len(in) && strings.TrimSpace(in[i+1]) == "```signature" {
				i++
				for i+1 < len(in) && strings.TrimSpace(in[i]) != "```" {
					i++
				}
			}
			continue
		}

		// Find existing signature fence if present right after the anchor
		fenceEnd := -1
		if i+1 < len(in) && strings.TrimSpace(in[i+1]) == "```signature" {
			for k := i + 1; k < len(in); k++ {
				if strings.TrimSpace(in[k]) == "```" {
					fenceEnd = k
					break
				}
			}
		}

		bodyStart := i + 1
		if fenceEnd >= 0 {
			bodyStart = fenceEnd + 1
		}

		j := endAt(in, bodyStart)

		// Check if AI rewrite should be attempted:
		// Stale, previously synced (oldSig != ""), sync == true, proseFn != nil, and end marker present (j >= 0)
		if proseFn == nil || oldSig == "" || j < 0 {
			// Mechanical refresh
			issues = append(issues, Issue{
				Line:  i + 1,
				Ref:   ref,
				Msg:   "stale: signature changed or never synced",
				Stale: true,
			})
			if proseFn != nil && oldSig != "" && j < 0 {
				issues = append(issues, Issue{
					Line:  i + 1,
					Ref:   ref,
					Msg:   "ai skipped: no @scribex:end marker",
					Stale: false,
					Code:  CodeAISkipped,
				})
			}
			out = append(out, fmt.Sprintf("<!-- @scribex:anchor(%s) sig=%s -->", ref, want), "```signature", sig, "```")
			if fenceEnd >= 0 {
				i = fenceEnd
			}
			continue
		}

		// AI rewrite path
		old := prose.StripAIMark(strings.Join(in[bodyStart:j], "\n"))
		var nw string
		var callErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					callErr = fmt.Errorf("panic: %v", r)
				}
			}()
			nw, callErr = proseFn(ref, sig, old)
		}()

		if callErr != nil {
			var v *prose.Violation
			if errors.As(callErr, &v) {
				detail := v.Rule
				if v.Detail != "" {
					detail += ": " + v.Detail
				}
				issues = append(issues, Issue{
					Line:  i + 1,
					Ref:   ref,
					Msg:   "ai rewrite rejected: " + detail,
					Stale: true,
					Code:  CodeAIRejected,
				})
			} else {
				issues = append(issues, Issue{
					Line:  i + 1,
					Ref:   ref,
					Msg:   "ai rewrite failed: " + callErr.Error(),
					Stale: true,
					Code:  CodeAIFailed,
				})
			}
			out = append(out, in[i:j]...)
			i = j - 1
			continue
		}

		if valErr := prose.Validate(old, nw); valErr != nil {
			var v *prose.Violation
			detail := valErr.Error()
			if errors.As(valErr, &v) {
				detail = v.Rule
				if v.Detail != "" {
					detail += ": " + v.Detail
				}
			}
			issues = append(issues, Issue{
				Line:  i + 1,
				Ref:   ref,
				Msg:   "ai rewrite rejected: " + detail,
				Stale: true,
				Code:  CodeAIRejected,
			})
			out = append(out, in[i:j]...)
			i = j - 1
			continue
		}

		// SUCCESS: emit new anchor line, new fence, cleaned prose, ai marker
		out = append(out, fmt.Sprintf("<!-- @scribex:anchor(%s) sig=%s -->", ref, want), "```signature", sig, "```")
		cleaned := prose.Clean(nw)
		if cleaned != "" {
			out = append(out, strings.Split(cleaned, "\n")...)
		}
		out = append(out, fmt.Sprintf("<!-- @scribex:ai sig=%s -->", want))
		i = j - 1
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
