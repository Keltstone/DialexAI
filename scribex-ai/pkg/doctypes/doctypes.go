// Package doctypes is the catalog of documentation kinds `scribex generate` can produce.
// User overrides live in .scribex/doctypes.json; built-ins are compiled in.
package doctypes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"scribex/pkg/fsx"
	"scribex/pkg/safepath"
)

const FileName = "doctypes.json"

// Type is one effective documentation kind (built-in merged with any override).
type Type struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Audience    string   `json:"audience"` // dev | qa | po | exec | ops
	Enabled     bool     `json:"enabled"`
	Granularity string   `json:"granularity"` // repo | module | package | feature | file | symbol
	Include     []string `json:"include,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	Output      string   `json:"output"`    // path template: {module} {package} {feature} {file}
	Grounding   string   `json:"grounding"` // symbol | derived | human
	Template    string   `json:"template,omitempty"`
	Inputs      []string `json:"inputs,omitempty"`
	IDPolicy    string   `json:"id_policy"`
	IDPattern   string   `json:"id_pattern,omitempty"`
	MaxCalls    int      `json:"max_calls"`
	Language    string   `json:"language"`
	Review      string   `json:"review,omitempty"` // "" | socratix
}

// Entry is the persisted override shape: pointer fields mean "unset", so a built-in can be tweaked
// field by field. A non-built-in id must set every required field.
type Entry struct {
	ID          string    `json:"id"`
	Title       *string   `json:"title,omitempty"`
	Audience    *string   `json:"audience,omitempty"`
	Enabled     *bool     `json:"enabled,omitempty"`
	Granularity *string   `json:"granularity,omitempty"`
	Include     *[]string `json:"include,omitempty"`
	Exclude     *[]string `json:"exclude,omitempty"`
	Output      *string   `json:"output,omitempty"`
	Grounding   *string   `json:"grounding,omitempty"`
	Template    *string   `json:"template,omitempty"`
	Inputs      *[]string `json:"inputs,omitempty"`
	IDPolicy    *string   `json:"id_policy,omitempty"`
	IDPattern   *string   `json:"id_pattern,omitempty"`
	MaxCalls    *int      `json:"max_calls,omitempty"`
	Language    *string   `json:"language,omitempty"`
	Review      *string   `json:"review,omitempty"`
}

// File is the on-disk document.
type File struct {
	Features map[string][]string `json:"features,omitempty"` // feature name -> globs
	Discover string              `json:"discover,omitempty"` // "" | "auto"
	Types    []Entry             `json:"types,omitempty"`
}

// Catalog is File merged over the built-ins.
type Catalog struct {
	File     File
	Types    []Type // built-ins first (stable order), then custom in file order
	Features map[string][]string
	Discover string
}

var (
	idRe       = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)
	langRe     = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
	featureRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,60}$`)
	placeholRe = regexp.MustCompile(`\{[a-z]+\}`)
	audiences  = set("dev", "qa", "po", "exec", "ops")
	grains     = set("repo", "module", "package", "feature", "file", "symbol")
	groundings = set("symbol", "derived", "human")
	policies   = set("none", "preserve")
	reviews    = set("", "socratix")
	validHolds = set("{module}", "{package}", "{feature}", "{file}")
	maxFile    = 256 << 10
)

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

// Path returns the absolute doctypes.json path under root.
func Path(root string) string {
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		dir = filepath.Join(root, safepath.DirName)
	}
	return filepath.Join(dir, FileName)
}

// Load reads .scribex/doctypes.json (missing file = built-ins only) and merges it over the built-ins.
func Load(root string) (Catalog, error) {
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		if os.IsNotExist(err) {
			c, _ := Build(File{})
			return c, nil
		}
		return Catalog{}, err
	}
	p := filepath.Join(dir, FileName)
	fi, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			c, _ := Build(File{})
			return c, nil
		}
		return Catalog{}, err
	}
	if !fi.Mode().IsRegular() {
		return Catalog{}, fmt.Errorf("%s must be a regular file", FileName)
	}
	if fi.Size() > int64(maxFile) {
		return Catalog{}, fmt.Errorf("%s exceeds %d bytes", FileName, maxFile)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Catalog{}, err
	}
	var f File
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Catalog{}, fmt.Errorf("%s: %w", FileName, err)
	}
	if dec.More() {
		return Catalog{}, fmt.Errorf("%s: trailing data", FileName)
	}
	c, probs := Build(f)
	if len(probs) > 0 {
		return c, fmt.Errorf("%s: %s", FileName, strings.Join(probs, "; "))
	}
	return c, nil
}

// Build merges f over the built-ins. Structural problems (duplicate ids, custom types missing required
// fields) are returned; field-value rules are checked by Validate.
func Build(f File) (Catalog, []string) {
	var probs []string
	c := Catalog{File: f, Features: f.Features, Discover: f.Discover}
	byID := map[string]int{}
	for _, b := range builtins() {
		byID[b.ID] = len(c.Types)
		c.Types = append(c.Types, b)
	}
	seen := map[string]bool{}
	for _, e := range f.Types {
		if seen[e.ID] {
			probs = append(probs, fmt.Sprintf("duplicate id %q", e.ID))
			continue
		}
		seen[e.ID] = true
		i, builtin := byID[e.ID]
		var t Type
		if builtin {
			t = c.Types[i]
		} else {
			t = Type{ID: e.ID, IDPolicy: "none", Language: "en"}
			for name, missing := range map[string]bool{
				"title": e.Title == nil, "audience": e.Audience == nil, "granularity": e.Granularity == nil,
				"output": e.Output == nil, "grounding": e.Grounding == nil,
			} {
				if missing {
					probs = append(probs, fmt.Sprintf("type %q: %s is required for a custom type", e.ID, name))
				}
			}
		}
		e.apply(&t)
		if builtin {
			c.Types[i] = t
		} else {
			byID[e.ID] = len(c.Types)
			c.Types = append(c.Types, t)
		}
	}
	sort.Strings(probs)
	return c, probs
}

func (e Entry) apply(t *Type) {
	if e.Title != nil {
		t.Title = *e.Title
	}
	if e.Audience != nil {
		t.Audience = *e.Audience
	}
	if e.Enabled != nil {
		t.Enabled = *e.Enabled
	}
	if e.Granularity != nil {
		t.Granularity = *e.Granularity
	}
	if e.Include != nil {
		t.Include = *e.Include
	}
	if e.Exclude != nil {
		t.Exclude = *e.Exclude
	}
	if e.Output != nil {
		t.Output = *e.Output
	}
	if e.Grounding != nil {
		t.Grounding = *e.Grounding
	}
	if e.Template != nil {
		t.Template = *e.Template
	}
	if e.Inputs != nil {
		t.Inputs = *e.Inputs
	}
	if e.IDPolicy != nil {
		t.IDPolicy = *e.IDPolicy
	}
	if e.IDPattern != nil {
		t.IDPattern = *e.IDPattern
	}
	if e.MaxCalls != nil {
		t.MaxCalls = *e.MaxCalls
	}
	if e.Language != nil {
		t.Language = *e.Language
	}
	if e.Review != nil {
		t.Review = *e.Review
	}
}

// Get returns the effective type with the given id.
func (c Catalog) Get(id string) (Type, bool) {
	for _, t := range c.Types {
		if t.ID == id {
			return t, true
		}
	}
	return Type{}, false
}

// Enabled returns enabled types in catalog order.
func (c Catalog) Enabled() (out []Type) {
	for _, t := range c.Types {
		if t.Enabled {
			out = append(out, t)
		}
	}
	return
}

// Validate returns every problem with the catalog, not just the first. root is used to confine and
// check template/input paths; enabled types must have existing template and inputs.
func Validate(root string, c Catalog) (probs []string) {
	ids := map[string]bool{}
	for _, b := range builtins() {
		ids[b.ID] = true
	}
	for _, e := range c.File.Types {
		if !idRe.MatchString(e.ID) {
			probs = append(probs, fmt.Sprintf("type %q: id must match %s", e.ID, idRe))
		}
	}
	for _, t := range c.Types {
		p := func(f string, a ...any) { probs = append(probs, "type "+t.ID+": "+fmt.Sprintf(f, a...)) }
		if !idRe.MatchString(t.ID) {
			continue // already reported
		}
		if strings.TrimSpace(t.Title) == "" || len(t.Title) > 120 {
			p("title is required (max 120 chars)")
		}
		if !audiences[t.Audience] {
			p("audience %q must be dev, qa, po, exec or ops", t.Audience)
		}
		if !grains[t.Granularity] {
			p("granularity %q must be repo, module, package, feature, file or symbol", t.Granularity)
		}
		if !groundings[t.Grounding] {
			p("grounding %q must be symbol, derived or human", t.Grounding)
		}
		if !policies[t.IDPolicy] {
			p("id_policy %q must be none or preserve", t.IDPolicy)
		}
		if !reviews[t.Review] {
			p("review %q must be empty or socratix", t.Review)
		}
		if !langRe.MatchString(t.Language) {
			p("language %q is not a language tag", t.Language)
		}
		if t.MaxCalls < 0 {
			p("max_calls must be >= 0")
		}
		if t.Grounding == "human" && (t.MaxCalls != 0 || t.Review != "") {
			p("grounding human never calls a model: max_calls must be 0 and review empty")
		}
		if t.IDPattern != "" {
			if _, err := regexp.Compile(t.IDPattern); err != nil || len(t.IDPattern) > 200 {
				p("id_pattern is not a valid regexp (max 200 chars)")
			}
		}
		if err := checkOutput(t); err != nil {
			p("%v", err)
		}
		for _, g := range append(append([]string(nil), t.Include...), t.Exclude...) {
			if err := CheckGlob(g); err != nil {
				p("glob %q: %v", g, err)
			}
		}
		if !ids[t.ID] && (t.Grounding == "derived" || t.Grounding == "human") && t.Template == "" {
			p("template is required for a custom %s type", t.Grounding)
		}
		for _, rel := range append([]string{t.Template}, t.Inputs...) {
			if rel == "" {
				continue
			}
			full, err := safepath.Join(root, rel)
			if err != nil {
				p("path %q: %v", rel, err)
				continue
			}
			if t.Enabled {
				if fi, err := os.Stat(full); err != nil || !fi.Mode().IsRegular() {
					p("path %q does not exist or is not a regular file", rel)
				}
			}
		}
		if t.Enabled && t.Granularity == "feature" && len(c.Features) == 0 && c.Discover != "auto" {
			p("granularity feature needs a non-empty features map or \"discover\": \"auto\"")
		}
	}
	for name, globs := range c.Features {
		if !featureRe.MatchString(name) {
			probs = append(probs, fmt.Sprintf("feature %q: invalid name", name))
		}
		if len(globs) == 0 {
			probs = append(probs, fmt.Sprintf("feature %q: needs at least one glob", name))
		}
		for _, g := range globs {
			if err := CheckGlob(g); err != nil {
				probs = append(probs, fmt.Sprintf("feature %q glob %q: %v", name, g, err))
			}
		}
	}
	if c.Discover != "" && c.Discover != "auto" {
		probs = append(probs, `discover must be "" or "auto"`)
	}
	sort.Strings(probs)
	return probs
}

func checkOutput(t Type) error {
	o := t.Output
	if o == "" {
		return errors.New("output is required")
	}
	if !strings.HasSuffix(o, ".md") {
		return errors.New("output must end in .md")
	}
	for _, ph := range placeholRe.FindAllString(o, -1) {
		if !validHolds[ph] {
			return fmt.Errorf("output uses unknown placeholder %s", ph)
		}
	}
	if strings.ContainsAny(placeholRe.ReplaceAllString(o, ""), "{}") {
		return errors.New("output has stray braces")
	}
	if strings.Contains(o, `\`) {
		return errors.New("output must use forward slashes")
	}
	sample := placeholRe.ReplaceAllString(o, "x")
	if err := safepath.CheckRel(sample); err != nil {
		return err
	}
	if path.Clean(sample) != sample || strings.HasPrefix(sample, "./") {
		return errors.New("output must be a clean relative path")
	}
	need := map[string]string{"module": "{module}", "package": "{package}", "feature": "{feature}", "file": "{file}", "symbol": "{file}"}[t.Granularity]
	if need != "" && !strings.Contains(o, need) {
		return fmt.Errorf("granularity %s needs %s in output, otherwise every unit writes the same file", t.Granularity, need)
	}
	if t.Grounding == "symbol" && strings.Contains(o, "{feature}") {
		return errors.New("a symbol-grounded type cannot use {feature}")
	}
	return nil
}

var safeSeg = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Resolve fills the output template. vals holds module/package/feature/file values; they are sanitised
// per path segment ("..", empty segments dropped), so no value can climb out of the output directory.
func Resolve(t Type, vals map[string]string) (string, error) {
	out := placeholRe.ReplaceAllStringFunc(t.Output, func(ph string) string {
		return cleanValue(vals[strings.Trim(ph, "{}")])
	})
	if strings.Contains(out, "{}") || strings.Contains(out, "//") {
		return "", fmt.Errorf("type %s: missing value for output %q", t.ID, t.Output)
	}
	if err := safepath.CheckRel(out); err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, ".md") {
		return "", fmt.Errorf("resolved output %q must end in .md", out)
	}
	return out, nil
}

func cleanValue(v string) string {
	v = strings.ReplaceAll(v, `\`, "/")
	var segs []string
	for _, s := range strings.Split(v, "/") {
		s = strings.Trim(safeSeg.ReplaceAllString(s, "-"), ".-")
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return "unnamed"
	}
	return strings.Join(segs, "/")
}

// CheckGlob validates a pattern in the dialect Match implements: path.Match per segment plus "**".
func CheckGlob(g string) error {
	if g == "" || len(g) > 300 || strings.ContainsRune(g, 0) {
		return errors.New("empty, oversized or contains NUL")
	}
	if strings.HasPrefix(g, "/") || strings.Contains(g, `\`) {
		return errors.New("must be a relative forward-slash pattern")
	}
	for _, s := range strings.Split(g, "/") {
		if s == ".." {
			return errors.New("must not contain ..")
		}
		if strings.Contains(s, "**") && s != "**" {
			return errors.New(`"**" must be a whole path segment`)
		}
		if _, err := path.Match(s, ""); err != nil {
			return err
		}
	}
	return nil
}

// Match reports whether slash-separated rel matches pattern ("**" matches zero or more segments).
func Match(pattern, rel string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegs(p, r []string) bool {
	for len(p) > 0 {
		if p[0] == "**" {
			for len(p) > 1 && p[1] == "**" {
				p = p[1:]
			}
			if len(p) == 1 {
				return true
			}
			for i := 0; i <= len(r); i++ {
				if matchSegs(p[1:], r[i:]) {
					return true
				}
			}
			return false
		}
		if len(r) == 0 {
			return false
		}
		if ok, err := path.Match(p[0], r[0]); err != nil || !ok {
			return false
		}
		p, r = p[1:], r[1:]
	}
	return len(r) == 0
}

// Selected reports whether rel is in scope for t (included and not excluded).
func (t Type) Selected(rel string) bool {
	in := len(t.Include) == 0
	for _, g := range t.Include {
		in = in || Match(g, rel)
	}
	for _, g := range t.Exclude {
		if Match(g, rel) {
			return false
		}
	}
	return in
}

// Save writes only the user overrides (never the full built-in set), atomically, in stable order.
func Save(root string, f File) error {
	c, probs := Build(f)
	if len(probs) > 0 {
		return fmt.Errorf("%s", strings.Join(probs, "; "))
	}
	if probs := Validate(root, c); len(probs) > 0 {
		return fmt.Errorf("%s", strings.Join(probs, "; "))
	}
	if _, err := safepath.ScribexDir(root, true); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ") // map keys are sorted by encoding/json
	if err != nil {
		return err
	}
	return fsx.WriteFileIn(root, filepath.Join(safepath.DirName, FileName), append(b, '\n'), 0o644)
}
