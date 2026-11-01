package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"scribex/pkg/anchors"
	"scribex/pkg/config"
	"scribex/pkg/derived"
	"scribex/pkg/doctypes"
	"scribex/pkg/fsx"
	"scribex/pkg/gitx"
	"scribex/pkg/safepath"
)

const (
	PlanVersion = 1
	excerptMax  = 12 << 10 // bytes of one source file sent in a prompt
	callMaxIn   = 60 << 10 // bytes of source per call
)

// Options selects what a plan covers. Nothing here is a secret.
type Options struct {
	Root     string
	Types    []string // explicit ids (may name disabled types); empty = all enabled
	Include  []string
	Exclude  []string
	MaxCalls int // 0 = no extra ceiling beyond ai.max_calls at apply time
	AI       config.AI
	RunID    string // "" = derived from the plan content
}

// Item is one output document for one doc type and unit.
type Item struct {
	Type               string           `json:"type"`
	Unit               string           `json:"unit"`
	Output             string           `json:"output"`
	Grounding          string           `json:"grounding"`
	Action             string           `json:"action"` // create | update | skip
	Reason             string           `json:"reason,omitempty"`
	Parser             map[string]int   `json:"parser,omitempty"` // ast|lexical -> files
	Files              []string         `json:"files,omitempty"`
	Syms               []string         `json:"syms,omitempty"` // file#Sym to (re)write, symbol grounding
	Sources            []derived.Source `json:"sources,omitempty"`
	Orphans            []string         `json:"orphans,omitempty"`
	HandwrittenOrphans []string         `json:"handwritten_orphans,omitempty"`
	Calls              int              `json:"calls"`
	Tokens             int              `json:"tokens"`
	Status             string           `json:"status"` // pending | done | skipped | failed
}

// Plan is the on-disk contract between the plan and apply phases. It carries no secrets and no timestamps.
type Plan struct {
	Version     int      `json:"version"`
	RunID       string   `json:"run_id"`
	Head        string   `json:"head,omitempty"`
	Backend     string   `json:"backend"`
	Model       string   `json:"model,omitempty"`
	Local       bool     `json:"local"`
	Items       []Item   `json:"items"`
	WouldSend   []string `json:"would_send"`
	Features    []string `json:"features,omitempty"`
	Skips       []string `json:"skips,omitempty"`
	TotalCalls  int      `json:"total_calls"`
	TotalTokens int      `json:"total_tokens"`
}

// IsLocal reports whether the backend keeps source on this machine. A vendor CLI is not local.
func IsLocal(a config.AI) bool { return a.Provider == "ollama" }

var (
	aiLine  = regexp.MustCompile(`^\s*<!--\s*@scribex:ai(?:\s+sig=[0-9a-fA-F]+)?\s*-->\s*$`)
	endLine = regexp.MustCompile(`^\s*<!--\s*@scribex:end\s*-->\s*$`)
)

type docState struct {
	anchors map[string]anchors.Anchor // ref -> anchor
	managed map[string]bool           // ref -> has an @scribex:ai marker in its section
	blocks  map[string]derived.Block
}

func readDoc(root, rel string) (md string, st docState, exists bool, err error) {
	p, err := safepath.Join(root, rel)
	if err != nil {
		return "", st, false, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return "", st, false, nil
	}
	if err != nil {
		return "", st, false, err
	}
	md = string(b)
	st = docState{anchors: map[string]anchors.Anchor{}, managed: map[string]bool{}, blocks: map[string]derived.Block{}}
	as := anchors.Parse(md)
	lines := strings.Split(md, "\n")
	for i, a := range as {
		ref := a.File + "#" + a.Sym
		st.anchors[ref] = a
		end := len(lines)
		if i+1 < len(as) {
			end = as[i+1].Line - 1
		}
		for l := a.Line; l < end && l < len(lines); l++ {
			if aiLine.MatchString(lines[l]) {
				st.managed[ref] = true
			}
			if endLine.MatchString(lines[l]) {
				break
			}
		}
	}
	for _, bl := range derived.Parse(md) {
		st.blocks[bl.ID] = bl
	}
	return md, st, true, nil
}

func (r *Repo) filtered(inc, exc []string) *Repo {
	out := &Repo{Root: r.Root, Skips: r.Skips, Tests: r.Tests, byRel: map[string]*SrcFile{}}
	for _, f := range r.Files {
		ok := len(inc) == 0
		for _, g := range inc {
			if doctypes.Match(g, f.Rel) {
				ok = true
			}
		}
		for _, g := range exc {
			if doctypes.Match(g, f.Rel) {
				ok = false
			}
		}
		if ok {
			out.Files = append(out.Files, f)
			out.byRel[f.Rel] = f
		}
	}
	return out
}

func relsOf(fs []*SrcFile) []string {
	var o []string
	for _, f := range fs {
		o = append(o, f.Rel)
	}
	return o
}

func parsers(fs []*SrcFile) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Parser]++
	}
	return m
}

func tokensOf(fs []*SrcFile) int {
	n := 0
	for _, f := range fs {
		s := int(f.Size)
		if s > excerptMax {
			s = excerptMax
		}
		n += s
	}
	return n/4 + 200
}

// MakePlan scans the repo and returns what a run would do. It makes no model call and writes nothing.
func MakePlan(root string, cat doctypes.Catalog, o Options) (*Plan, error) {
	repo, err := ScanRepo(root)
	if err != nil {
		return nil, err
	}
	repo = repo.filtered(o.Include, o.Exclude)
	var types []doctypes.Type
	if len(o.Types) > 0 {
		for _, id := range o.Types {
			t, ok := cat.Get(id)
			if !ok {
				return nil, fmt.Errorf("unknown doc type %q", id)
			}
			types = append(types, t)
		}
	} else {
		types = cat.Enabled()
	}
	p := &Plan{Version: PlanVersion, Backend: o.AI.Provider, Model: o.AI.Model, Local: IsLocal(o.AI), Skips: repo.Skips}
	if o.AI.Provider == "cli" {
		p.Backend = "cli:" + o.AI.CLICommand
	}
	if b, err := gitx.Run(root, "rev-parse", "HEAD"); err == nil {
		p.Head = strings.TrimSpace(string(b))
	}
	send := map[string]bool{}
	for _, t := range types {
		var units []Unit
		units = unitsFor(t, cat, repo)
		for _, u := range units {
			out, err := doctypes.Resolve(t, u.Vals)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", t.ID, u.Key, err)
			}
			it := Item{Type: t.ID, Unit: u.Key, Output: out, Grounding: t.Grounding, Files: relsOf(u.Files), Parser: parsers(u.Files), Status: "pending"}
			md, st, exists, err := readDoc(root, out)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", out, err)
			}
			_ = md
			switch t.Grounding {
			case "symbol":
				planSymbols(&it, u, st, exists, repo)
			case "derived":
				planDerived(&it, t, u, st, root, repo)
			case "human":
				if exists {
					it.Action, it.Reason = "skip", "exists; Scribex never edits human docs"
				} else {
					it.Action = "create"
				}
			}
			if it.Action == "skip" {
				it.Status = "skipped"
			}
			if it.Calls > 0 {
				for _, f := range it.Files {
					send[f] = true
				}
				for _, s := range it.Sources {
					send[s.Path] = true
				}
			}
			p.TotalCalls += it.Calls
			p.TotalTokens += it.Tokens
			p.Items = append(p.Items, it)
		}
	}
	for f := range send {
		p.WouldSend = append(p.WouldSend, f)
	}
	sort.Strings(p.WouldSend)
	p.Features = DiscoverFeatures(cat, repo)
	if o.RunID != "" {
		p.RunID = o.RunID
	} else {
		b, _ := json.Marshal(p.Items)
		p.RunID = "r" + hash12(append(b, p.Head...))
	}
	return p, nil
}

func planSymbols(it *Item, u Unit, st docState, exists bool, repo *Repo) {
	create, update := 0, 0
	for _, f := range u.Files {
		for _, s := range f.Syms {
			ref := f.Rel + "#" + s.Key
			a, ok := st.anchors[ref]
			switch {
			case !ok:
				it.Syms = append(it.Syms, ref)
				create++
			case st.managed[ref] && !anchors.SigMatches(a.Sig, anchors.Hash(s.Sig)):
				it.Syms = append(it.Syms, ref)
				update++
			}
		}
	}
	for ref, a := range st.anchors {
		existsInCode := false
		if f, ok := repo.File(a.File); ok {
			for _, s := range f.Syms {
				if s.Key == a.Sym {
					existsInCode = true
					break
				}
			}
		}
		if !existsInCode {
			if st.managed[ref] {
				it.Orphans = append(it.Orphans, ref)
			} else {
				it.HandwrittenOrphans = append(it.HandwrittenOrphans, ref)
			}
		}
	}
	sort.Strings(it.Orphans)
	sort.Strings(it.HandwrittenOrphans)
	switch {
	case len(it.Syms) == 0 && len(it.Orphans) == 0:
		it.Action, it.Reason = "skip", "all symbols documented and current"
	case exists && update > 0 && create == 0:
		it.Action = "update"
	case exists:
		it.Action = "update"
	default:
		it.Action = "create"
	}
	it.Calls = (len(it.Syms) + maxSymbolsPer - 1) / maxSymbolsPer
	if it.Calls > 0 {
		it.Tokens = tokensOf(u.Files)
	}
}

func planDerived(it *Item, t doctypes.Type, u Unit, st docState, root string, repo *Repo) {
	id := t.ID + ":" + u.Key
	var srcs []derived.Source
	rels := relsOf(u.Files)
	rels = append(rels, t.Inputs...)
	if t.ID == "test-requirements" {
		rels = append(rels, repo.TestsFor(u.Files)...)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if len(srcs) >= derived.MaxSources {
			break
		}
		h, err := derived.HashFile(root, rel)
		if err != nil {
			continue
		}
		srcs = append(srcs, derived.Source{Path: rel, Hash: h})
	}
	it.Sources = srcs
	old, ok := st.blocks[id]
	if ok && sameSources(old.Sources, srcs) {
		it.Action, it.Reason = "skip", "sources unchanged"
		return
	}
	it.Action = "create"
	if ok {
		it.Action = "update"
	}
	it.Calls = 1
	it.Tokens = tokensOf(u.Files)
	if t.Review == "socratix" {
		it.Calls++
	}
}

func sameSources(a, b []derived.Source) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RunDir is the repo-relative directory of a run's files.
func RunDir(id string) string { return ".scribex/runs/" + id }

var runIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidRunID guards ids that arrive from flags, URLs or plan files.
func ValidRunID(id string) bool { return runIDRe.MatchString(id) }

// Save writes plan.json and plan.md under .scribex/runs/<id>/ and keeps that directory out of git.
func Save(root string, p *Plan) error {
	if !ValidRunID(p.RunID) {
		return fmt.Errorf("invalid run id %q", p.RunID)
	}
	if _, err := safepath.ScribexDir(root, true); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := fsx.WriteFileIn(root, RunDir(p.RunID)+"/plan.json", append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := fsx.WriteFileIn(root, RunDir(p.RunID)+"/plan.md", []byte(p.Markdown()), 0o644); err != nil {
		return err
	}
	excludeRuns(root)
	return nil
}

// excludeRuns adds .scribex/runs/ to .git/info/exclude (unversioned, so the tree stays clean). Best effort.
func excludeRuns(root string) {
	out, err := gitx.Run(root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	cur, _ := os.ReadFile(p)
	if strings.Contains(string(cur), ".scribex/runs/") {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(cur) > 0 && !strings.HasSuffix(string(cur), "\n") {
		f.WriteString("\n")
	}
	f.WriteString(".scribex/runs/\n")
}

// LoadPlan reads a saved plan.
func LoadPlan(root, id string) (*Plan, error) {
	if !ValidRunID(id) {
		return nil, fmt.Errorf("invalid run id %q", id)
	}
	p, err := safepath.Join(root, RunDir(id)+"/plan.json")
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var pl Plan
	if err := json.Unmarshal(b, &pl); err != nil {
		return nil, fmt.Errorf("plan.json: %w", err)
	}
	if pl.Version != PlanVersion || pl.RunID != id {
		return nil, fmt.Errorf("plan.json: unsupported version or id mismatch")
	}
	return &pl, nil
}

// Markdown renders the human-readable plan.
func (p *Plan) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Scribex generate plan %s\n\nBackend: %s", p.RunID, p.Backend)
	if p.Model != "" {
		fmt.Fprintf(&b, " (%s)", p.Model)
	}
	if p.Local {
		b.WriteString(" - local, no source leaves this machine\n")
	} else {
		b.WriteString(" - NOT local: files below leave this machine; apply needs --confirm-send\n")
	}
	fmt.Fprintf(&b, "\nEstimated calls: %d, input tokens: ~%d\n\n| Type | Unit | Action | Output | Calls | Parser |\n|---|---|---|---|---|---|\n", p.TotalCalls, p.TotalTokens)
	for _, it := range p.Items {
		ps := []string{}
		for _, k := range []string{"ast", "lexical"} {
			if n := it.Parser[k]; n > 0 {
				ps = append(ps, fmt.Sprintf("%s:%d", k, n))
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %s |\n", it.Type, it.Unit, it.Action, it.Output, it.Calls, strings.Join(ps, " "))
	}
	if len(p.Features) > 0 {
		fmt.Fprintf(&b, "\nDiscovered features: %s\n", strings.Join(p.Features, ", "))
	}
	var allOrphans []string
	for _, it := range p.Items {
		allOrphans = append(allOrphans, it.Orphans...)
	}
	if len(allOrphans) > 0 {
		sort.Strings(allOrphans)
		b.WriteString("\nOrphans (use -prune to remove):\n")
		for _, o := range allOrphans {
			fmt.Fprintf(&b, "- %s\n", o)
		}
	}
	var allHWOrphans []string
	for _, it := range p.Items {
		allHWOrphans = append(allHWOrphans, it.HandwrittenOrphans...)
	}
	if len(allHWOrphans) > 0 {
		sort.Strings(allHWOrphans)
		b.WriteString("\nHand-written orphans (report only):\n")
		for _, o := range allHWOrphans {
			fmt.Fprintf(&b, "- %s\n", o)
		}
	}
	fmt.Fprintf(&b, "\nFiles that would be sent (%d):\n", len(p.WouldSend))
	for _, f := range p.WouldSend {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	if len(p.Skips) > 0 {
		b.WriteString("\nSkipped while scanning:\n")
		for _, s := range p.Skips {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}
	return b.String()
}
