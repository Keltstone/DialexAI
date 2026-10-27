// Package anchors parses `<!-- @scribex:anchor(file#Symbol) -->` blocks and checks/syncs them against code.
package anchors

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"scribex/pkg/prose"
)

// Lookup returns the current signature of file#sym, ok=false when missing.
type Lookup func(file, sym string) (sig string, ok bool, err error)

const (
	CodeAIRejected   = "ai-rejected"
	CodeAIFailed     = "ai-failed"
	CodeAISkipped    = "ai-skipped"
	CodeWriteRefused = "write-refused"
)

type Issue struct {
	Line  int // 1-based
	Ref   string
	Msg   string
	Stale bool   // fixable by sync; false = dead anchor
	Code  string // ai-rejected | ai-failed | ai-skipped | write-refused
}

func (i Issue) String() string { return fmt.Sprintf(":%d %s: %s", i.Line, i.Ref, i.Msg) }

func Hash(sig string) string {
	h := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(h[:8])
}

// SigMatches reports whether stored matches want (16 hex chars).
// It returns true if stored == want, OR if stored is the legacy 8-char hash
// (len(stored) == 8 and strings.HasPrefix(want, stored)).
// Any other length (1–7, 9–15, >16) returns false.
func SigMatches(stored, want string) bool {
	if len(stored) == 16 {
		return stored == want
	}
	if len(stored) == 8 {
		return strings.HasPrefix(want, stored)
	}
	return false
}

// Prose rewrites an anchor's prose for a changed signature. Only called for anchors that were
// synced before (sig= present) and whose signature changed, so first-time syncs never spend tokens.
type Prose func(ref, sig, old string) (string, error)

const endMark = "<!-- @scribex:end -->"

// Process walks md; returns issues and, when sync is true, the rewritten text.
func Process(md string, look Lookup, sync bool, proseFn Prose) ([]Issue, string) {
	eol := "\n"
	if strings.Count(md, "\r\n")*2 > strings.Count(md, "\n") {
		eol = "\r\n"
	}
	rSuffix := ""
	if eol == "\r\n" {
		rSuffix = "\r"
	}

	in := strings.Split(md, "\n")
	inFence := InFence(in)
	var out []string
	var issues []Issue
	for i := 0; i < len(in); i++ {
		if inFence[i] {
			out = append(out, in[i])
			continue
		}
		lineForMatch := in[i]
		hasBOM := false
		if i == 0 && strings.HasPrefix(lineForMatch, "\uFEFF") {
			hasBOM = true
			lineForMatch = strings.TrimPrefix(lineForMatch, "\uFEFF")
		}
		m := open.FindStringSubmatch(lineForMatch)
		if m == nil {
			out = append(out, in[i])
			continue
		}
		file, sym := m[1], m[2]
		ref := file + "#" + sym
		if err := validateAnchor(file, sym); err != nil {
			issues = append(issues, Issue{
				Line:  i + 1,
				Ref:   ref,
				Msg:   "cannot analyse " + file + ": " + err.Error(),
				Stale: false,
			})
			out = append(out, in[i])
			continue
		}
		sig, ok, err := look(file, sym)
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
		isStale := !SigMatches(oldSig, want)

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

		formatAnchor := func() string {
			line := fmt.Sprintf("<!-- @scribex:anchor(%s) sig=%s -->", ref, want)
			if hasBOM {
				line = "\uFEFF" + line
			}
			return line + rSuffix
		}

		appendSignatureFence := func() {
			out = append(out, formatAnchor(), "```signature"+rSuffix)
			sigClean := strings.ReplaceAll(sig, "\r\n", "\n")
			for _, sl := range strings.Split(sigClean, "\n") {
				out = append(out, sl+rSuffix)
			}
			out = append(out, "```"+rSuffix)
		}

		// When sync is true:
		// If not stale: refresh signature fence if present to remain idempotent
		if !isStale {
			appendSignatureFence()
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
			appendSignatureFence()
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
		appendSignatureFence()
		cleaned := prose.Clean(nw)
		if cleaned != "" {
			for _, pl := range strings.Split(strings.ReplaceAll(cleaned, "\r\n", "\n"), "\n") {
				out = append(out, pl+rSuffix)
			}
		}
		out = append(out, fmt.Sprintf("<!-- @scribex:ai sig=%s -->", want)+rSuffix)
		i = j - 1
	}
	return issues, strings.Join(out, "\n")
}

// Count returns the number of anchors in md.
func Count(md string) int {
	return len(Parse(md))
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
