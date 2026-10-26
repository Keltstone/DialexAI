// Package docs is the repo-level engine: anchor checks, link scavenging, contract deltas.
package docs

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"scribex/pkg/anchors"
	"scribex/pkg/fsx"
	"scribex/pkg/gitx"
	"scribex/pkg/mdsafe"
	"scribex/pkg/safepath"
	"scribex/pkg/symbols"
)

type Issue struct {
	File string
	anchors.Issue
}

func (i Issue) String() string { return i.File + i.Issue.String() }

type Unknown struct{ File, Reason string }

type Report struct {
	Total   int // anchors seen
	Issues  []Issue
	Unknown []Unknown
	Skipped []string // symlinks skipped
	Changed []string // repo-relative, sorted files actually written
}

// Delta is one public-symbol change between base and the working tree.
type Delta struct{ File, Sym, Kind string } // Kind: added | changed | removed

// Breaking reports whether consumers of the symbol can be affected.
func (d Delta) Breaking() bool { return d.Kind != "added" }

type DeltaResult struct {
	Deltas  []Delta
	Unknown []Unknown // files we could not analyse but cannot ignore
	Skipped []string  // submodules etc. (informational)
}

// FilesWithSkips lists markdown files under root and any skipped symlink paths.
func FilesWithSkips(root string) (files, skipped []string) {
	walkRoot := root
	if fi, err := os.Lstat(root); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if real, err := filepath.EvalSymlinks(root); err == nil {
			walkRoot = real
		}
	}

	filepath.WalkDir(walkRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		outPath := p
		if walkRoot != root {
			if rel, err := filepath.Rel(walkRoot, p); err == nil {
				outPath = filepath.Join(root, rel)
			}
		}

		if d.IsDir() && p != walkRoot {
			n := d.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" || n == "vendor" {
				return filepath.SkipDir
			}
		}
		if p != walkRoot && d.Type()&os.ModeSymlink != 0 {
			skipped = append(skipped, outPath)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			rel, err := filepath.Rel(walkRoot, p)
			if err != nil {
				return nil
			}
			if _, err := safepath.Join(root, rel); err != nil {
				return nil
			}
			files = append(files, outPath)
		}
		return nil
	})
	sort.Strings(files)
	sort.Strings(skipped)
	return
}

// Files lists markdown files under root.
func Files(root string) []string {
	files, _ := FilesWithSkips(root)
	return files
}

// Symbols returns the public symbols of a repo-relative file.
func Symbols(root, file string) (map[string]string, error) {
	p, err := safepath.Join(root, file)
	if err != nil {
		return nil, fmt.Errorf("anchor path %q: %w", file, err)
	}
	src, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return symbols.Extract(file, src)
}

// Check validates (and with sync, repairs) every anchor under root. Stale issues fixed by sync are not reported.
func Check(root string, sync bool, prose anchors.Prose) (Report, error) {
	type fileResult struct {
		syms map[string]string
		err  error
	}
	cache := map[string]fileResult{}
	look := func(file, sym string) (string, bool, error) {
		res, ok := cache[file]
		if !ok {
			syms, err := Symbols(root, file)
			res = fileResult{syms: syms, err: err}
			cache[file] = res
		}
		if res.err != nil {
			return "", false, res.err
		}
		v, ok := res.syms[sym]
		return v, ok, nil
	}
	var r Report
	files, skipped := FilesWithSkips(root)
	r.Skipped = skipped
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return r, err
		}
		r.Total += anchors.Count(string(b))
		issues, out := anchors.Process(string(b), look, sync, prose)
		for _, i := range issues {
			if !(sync && i.Stale && i.Code == "") {
				r.Issues = append(r.Issues, Issue{f, i})
			}
		}
		if sync && out != string(b) {
			rel, err := filepath.Rel(root, f)
			if err != nil {
				return r, err
			}
			safeP, err := safepath.Join(root, rel)
			if err != nil {
				return r, err
			}
			realF, err := filepath.EvalSymlinks(f)
			if err != nil && !os.IsNotExist(err) {
				return r, err
			}
			if realF != "" && safeP != realF {
				return r, fmt.Errorf("refusing to write to %s: resolves to %s, expected %s", f, realF, safeP)
			}
			if err := fsx.WriteFileIn(root, rel, []byte(out), 0o644); err != nil {
				r.Issues = append(r.Issues, Issue{
					File: f,
					Issue: anchors.Issue{
						Ref:   rel,
						Msg:   "write refused: " + err.Error(),
						Stale: false,
						Code:  anchors.CodeWriteRefused,
					},
				})
				continue
			}
			r.Changed = append(r.Changed, rel)
		}
	}
	sort.Strings(r.Changed)
	return r, nil
}

var linkRe = regexp.MustCompile(`\]\(([^)]+)\)`)
var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// Links reports markdown links to repo-relative targets that do not exist or escape root.
func Links(root string) (out []Issue) {
	for _, f := range Files(root) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		inFence := anchors.InFence(lines)
		for n, l := range lines {
			if inFence[n] {
				continue
			}
			for _, m := range linkRe.FindAllStringSubmatch(l, -1) {
				raw := strings.TrimSpace(m[1])
				t := raw
				if fields := strings.Fields(raw); len(fields) > 0 {
					t = fields[0]
				}
				ref := t

				// 1. Strip ?query and #fragment
				if idx := strings.IndexAny(t, "?#"); idx >= 0 {
					t = t[:idx]
				}
				// 2. Percent-decode
				decoded, err := url.PathUnescape(t)
				if err != nil {
					out = append(out, Issue{f, anchors.Issue{Line: n + 1, Ref: ref, Msg: "broken link"}})
					continue
				}
				t = decoded
				// 3. Empty after strip -> skip
				if t == "" {
					continue
				}
				// 4. Scheme or starting //
				if schemeRe.MatchString(t) || strings.HasPrefix(t, "//") {
					continue
				}
				// 5. Leading / means repo root
				var rel string
				if strings.HasPrefix(t, "/") {
					rel = strings.TrimPrefix(t, "/")
				} else {
					dirRel, err := filepath.Rel(root, filepath.Dir(f))
					if err != nil {
						dirRel = ""
					}
					rel = filepath.Join(dirRel, t)
				}
				// 6. Resolve with safepath.Join(root, rel)
				targetPath, err := safepath.Join(root, rel)
				if err != nil {
					out = append(out, Issue{f, anchors.Issue{Line: n + 1, Ref: ref, Msg: "link escapes repository"}})
					continue
				}
				// 7. os.Lstat (do not follow final symlink to outside)
				if _, err := os.Lstat(targetPath); err != nil {
					out = append(out, Issue{f, anchors.Issue{Line: n + 1, Ref: ref, Msg: "broken link"}})
					continue
				}
			}
		}
	}
	return
}

// Refs maps "file#Sym" to the doc files anchoring it.
func Refs(root string) map[string][]string {
	out := map[string][]string{}
	for _, f := range Files(root) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, a := range anchors.Parse(string(b)) {
			key := a.File + "#" + a.Sym
			out[key] = append(out[key], f)
		}
	}
	return out
}

// Deltas diffs public symbols of every file changed since base.
func Deltas(root, base string) (DeltaResult, error) {
	if err := gitx.ValidRef(base); err != nil {
		return DeltaResult{}, fmt.Errorf("resolve base %q: %w", base, err)
	}
	sha, err := gitx.ResolveCommit(root, base)
	if err != nil {
		if shallow, _ := gitx.IsShallow(root); shallow {
			return DeltaResult{}, fmt.Errorf("shallow clone: cannot compute merge-base with %s; fetch full history (GitHub: fetch-depth: 0; GitLab: GIT_DEPTH=0)", base)
		}
		return DeltaResult{}, fmt.Errorf("resolve base %q: %w", base, err)
	}

	mbBytes, err := gitx.Run(root, "merge-base", sha, "HEAD")
	if err != nil {
		if shallow, _ := gitx.IsShallow(root); shallow {
			return DeltaResult{}, fmt.Errorf("shallow clone: cannot compute merge-base with %s; fetch full history (GitHub: fetch-depth: 0; GitLab: GIT_DEPTH=0)", base)
		}
		return DeltaResult{}, fmt.Errorf("merge-base %s HEAD: %w", base, err)
	}
	mb := strings.TrimSpace(string(mbBytes))

	diffOut, err := gitx.Run(root, "diff", "--relative", "--raw", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", mb, "--")
	if err != nil {
		return DeltaResult{}, fmt.Errorf("git diff: %w", err)
	}

	var skipped []string
	type fileChange struct {
		status string
	}
	files := make(map[string]fileChange)

	if len(diffOut) > 0 {
		records := bytes.Split(diffOut, []byte{0})
		for i := 0; i+1 < len(records); i += 2 {
			hdr := string(records[i])
			path := string(records[i+1])
			if path == "" {
				continue
			}
			fields := strings.Fields(strings.TrimPrefix(hdr, ":"))
			if len(fields) < 5 {
				return DeltaResult{}, fmt.Errorf("malformed diff record header %q for %s", hdr, path)
			}
			srcmode, dstmode, status := fields[0], fields[1], fields[4]
			if srcmode == "160000" || dstmode == "160000" {
				skipped = append(skipped, path)
				continue
			}
			switch status {
			case "A", "M", "D", "T":
				files[path] = fileChange{status: status}
			default:
				return DeltaResult{}, fmt.Errorf("unsupported diff status %q for %s", status, path)
			}
		}
	}

	untrackedOut, err := gitx.Run(root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return DeltaResult{}, fmt.Errorf("git ls-files: %w", err)
	}
	if len(untrackedOut) > 0 {
		for _, p := range bytes.Split(untrackedOut, []byte{0}) {
			if len(p) == 0 {
				continue
			}
			path := string(p)
			if _, exists := files[path]; !exists {
				files[path] = fileChange{status: "A"}
			}
		}
	}

	var sortedPaths []string
	for p := range files {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)
	sort.Strings(skipped)

	var referencedFiles map[string]bool
	getReferenced := func() map[string]bool {
		if referencedFiles != nil {
			return referencedFiles
		}
		referencedFiles = make(map[string]bool)
		for ref := range Refs(root) {
			f, _, _ := strings.Cut(ref, "#")
			referencedFiles[f] = true
		}
		return referencedFiles
	}

	var res DeltaResult
	res.Skipped = skipped

	for _, f := range sortedPaths {
		change := files[f]
		if !symbols.Supported(f) {
			if getReferenced()[f] {
				res.Unknown = append(res.Unknown, Unknown{
					File:   f,
					Reason: "unsupported language referenced by an anchor",
				})
			}
			continue
		}

		var oldSyms map[string]string
		var oldErr error
		if change.status == "A" {
			oldSyms = make(map[string]string)
		} else {
			oldSrc, err := gitx.Run(root, "show", mb+":./"+f)
			if err != nil {
				return DeltaResult{}, fmt.Errorf("git show %s:./%s: %w", mb, f, err)
			}
			oldSyms, oldErr = symbols.Extract(f, oldSrc)
		}

		var newSyms map[string]string
		var newErr error
		if change.status == "D" {
			newSyms = make(map[string]string)
		} else {
			p, err := safepath.Join(root, f)
			if err != nil {
				newErr = err
			} else {
				newSrc, err := os.ReadFile(p)
				if err != nil {
					newErr = err
				} else {
					newSyms, newErr = symbols.Extract(f, newSrc)
				}
			}
		}

		if oldErr != nil {
			res.Unknown = append(res.Unknown, Unknown{File: f, Reason: oldErr.Error()})
			continue
		}
		if newErr != nil {
			res.Unknown = append(res.Unknown, Unknown{File: f, Reason: newErr.Error()})
			continue
		}

		for k, v := range newSyms {
			if ov, ok := oldSyms[k]; !ok {
				res.Deltas = append(res.Deltas, Delta{f, k, "added"})
			} else if ov != v {
				res.Deltas = append(res.Deltas, Delta{f, k, "changed"})
			}
		}
		for k := range oldSyms {
			if _, ok := newSyms[k]; !ok {
				res.Deltas = append(res.Deltas, Delta{f, k, "removed"})
			}
		}
	}

	sort.Slice(res.Deltas, func(i, j int) bool {
		return res.Deltas[i].File+res.Deltas[i].Sym < res.Deltas[j].File+res.Deltas[j].Sym
	})

	sort.Slice(res.Unknown, func(i, j int) bool {
		return res.Unknown[i].File < res.Unknown[j].File
	})

	return res, nil
}

// Markdown renders a PR-comment body with length cap and neutralised fields.
func (r Report) Markdown(extra ...Issue) string {
	all := append(append([]Issue{}, r.Issues...), extra...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		if all[i].Ref != all[j].Ref {
			return all[i].Ref < all[j].Ref
		}
		return all[i].Msg < all[j].Msg
	})

	unknowns := append([]Unknown{}, r.Unknown...)
	sort.Slice(unknowns, func(i, j int) bool {
		if unknowns[i].File != unknowns[j].File {
			return unknowns[i].File < unknowns[j].File
		}
		return unknowns[i].Reason < unknowns[j].Reason
	})

	var unknownSection string
	if len(unknowns) > 0 {
		var ub strings.Builder
		ub.WriteString("\n\nUnanalysed files:\n")
		for _, u := range unknowns {
			ub.WriteString("- " + mdsafe.Code(u.File) + ": " + mdsafe.Text(u.Reason) + "\n")
		}
		unknownSection = ub.String()
	}

	var b strings.Builder
	b.WriteString(ReportHeader)
	b.WriteString("\n\n")

	if len(all) == 0 {
		fmt.Fprintf(&b, "Scribex: docs in sync (%d anchors checked).", r.Total)
		b.WriteString(unknownSection)
		return b.String()
	}

	header := fmt.Sprintf("Scribex found %d doc issue(s) (%d anchors):\n\n", len(all), r.Total)
	b.WriteString(header)

	const syncFooter = "\nRun `scribex sync` to refresh stale anchors."
	maxPayload := MaxBodyLen - len(MarkerV1) - 1 // 59973

	shown := 0
	for _, iss := range all {
		codeFile := mdsafe.Code(iss.File)
		codeRef := mdsafe.Code(iss.Ref)
		textMsg := mdsafe.Text(iss.Msg)
		var line string
		if iss.Ref != "" {
			line = fmt.Sprintf("- %s line %d %s: %s\n", codeFile, iss.Line, codeRef, textMsg)
		} else {
			line = fmt.Sprintf("- %s line %d: %s\n", codeFile, iss.Line, textMsg)
		}

		remainingIfCut := len(all) - shown
		footerIfCut := fmt.Sprintf("… %d more findings not shown. Run `scribex check` locally for the full report.\n", remainingIfCut)

		if b.Len()+len(line)+len(footerIfCut)+len(syncFooter)+len(unknownSection) > maxPayload {
			b.WriteString(footerIfCut)
			break
		}
		b.WriteString(line)
		shown++
	}

	b.WriteString(syncFooter)
	b.WriteString(unknownSection)
	return b.String()
}
