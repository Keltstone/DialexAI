package generate

import (
	"path"
	"sort"
	"strings"

	"scribex/pkg/doctypes"
)

// Unit is one output document's worth of source: a file, package, module, feature or the whole repo.
type Unit struct {
	Key   string
	Vals  map[string]string // placeholder values for doctypes.Resolve
	Files []*SrcFile
}

var featureMarkers = map[string]bool{"feature": true, "features": true, "screen": true, "screens": true, "presentation": true}

// autoFeature names the feature a file belongs to when discovery is "auto": the path segment after the
// first feature/screen/presentation directory, else the module's first sub-directory, else the module.
func autoFeature(f *SrcFile) string {
	segs := strings.Split(path.Dir(f.Rel), "/")
	for i, s := range segs {
		if featureMarkers[strings.ToLower(s)] && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	if f.Module != "root" && strings.HasPrefix(f.Rel, f.Module+"/") {
		rest := strings.TrimPrefix(f.Rel, f.Module+"/")
		if i := strings.IndexByte(rest, '/'); i > 0 {
			return rest[:i]
		}
	}
	return f.Module
}

// DiscoverFeatures lists the features a catalog would produce for repo (for the plan's approval list).
func DiscoverFeatures(cat doctypes.Catalog, repo *Repo) []string {
	set := map[string]bool{}
	for _, u := range featureUnits(cat, repo, doctypes.Type{}) {
		set[u.Key] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func featureUnits(cat doctypes.Catalog, repo *Repo, t doctypes.Type) []Unit {
	by := map[string][]*SrcFile{}
	for _, f := range repo.Files {
		if t.ID != "" && !t.Selected(f.Rel) {
			continue
		}
		if len(cat.Features) > 0 {
			for name, globs := range cat.Features {
				for _, g := range globs {
					if doctypes.Match(g, f.Rel) {
						by[name] = append(by[name], f)
						break
					}
				}
			}
		} else if cat.Discover == "auto" {
			n := autoFeature(f)
			by[n] = append(by[n], f)
		}
	}
	var out []Unit
	for name, fs := range by {
		out = append(out, Unit{Key: name, Vals: map[string]string{"feature": name}, Files: fs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// unitsFor groups the repo's files into the units a doc type produces, in stable order. Files outside
// the type's include/exclude scope never appear. Symbol-grounded units without public symbols are dropped.
func unitsFor(t doctypes.Type, cat doctypes.Catalog, repo *Repo) []Unit {
	var files []*SrcFile
	for _, f := range repo.Files {
		if t.Selected(f.Rel) {
			files = append(files, f)
		}
	}
	var units []Unit
	group := func(key func(*SrcFile) (string, string)) {
		by := map[string]*Unit{}
		var order []string
		for _, f := range files {
			k, ph := key(f)
			u, ok := by[k]
			if !ok {
				u = &Unit{Key: k, Vals: map[string]string{ph: k}}
				by[k] = u
				order = append(order, k)
			}
			u.Files = append(u.Files, f)
		}
		sort.Strings(order)
		for _, k := range order {
			units = append(units, *by[k])
		}
	}
	switch t.Granularity {
	case "file", "symbol":
		group(func(f *SrcFile) (string, string) { return f.Rel, "file" })
	case "package":
		group(func(f *SrcFile) (string, string) {
			d := path.Dir(f.Rel)
			if d == "." {
				d = "root"
			}
			return d, "package"
		})
	case "module":
		group(func(f *SrcFile) (string, string) { return f.Module, "module" })
	case "feature":
		units = featureUnits(cat, repo, t)
	case "repo":
		if len(files) > 0 {
			units = []Unit{{Key: "repo", Vals: map[string]string{}, Files: files}}
		}
	}
	if t.Grounding == "symbol" {
		var keep []Unit
		for _, u := range units {
			n := 0
			for _, f := range u.Files {
				n += len(f.Syms)
			}
			if n > 0 {
				keep = append(keep, u)
			}
		}
		units = keep
	}
	return units
}
