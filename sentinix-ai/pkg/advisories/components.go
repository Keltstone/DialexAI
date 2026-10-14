// Package advisories builds an SBOM from a repo's dependency files and matches it against OSV.
package advisories

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Component struct {
	Ecosystem string `json:"ecosystem"` // OSV names: Go, npm, PyPI, Maven
	Name      string `json:"name"`      // Maven: group:artifact
	Version   string `json:"version"`
	File      string `json:"file"`
}

var (
	gradleDep   = regexp.MustCompile(`["']([\w.\-]+):([\w.\-]+):([\w.\-]+)["']`) // versions with $ never match
	tomlVersion = regexp.MustCompile(`^\s*([\w.\-]+)\s*=\s*"([^"]+)"`)
	tomlLib     = regexp.MustCompile(`module\s*=\s*"([^":]+:[^":]+)"[^}]*?version(\.ref)?\s*=\s*"([^"]+)"`)
	tomlLib2    = regexp.MustCompile(`group\s*=\s*"([^"]+)"\s*,\s*name\s*=\s*"([^"]+)"[^}]*?version(\.ref)?\s*=\s*"([^"]+)"`)
	pomDep      = regexp.MustCompile(`(?s)<dependency>\s*<groupId>([^<$]+)</groupId>\s*<artifactId>([^<]+)</artifactId>\s*<version>([^<$]+)</version>`)
	pyReq       = regexp.MustCompile(`^([A-Za-z0-9_.\-]+)==([A-Za-z0-9_.\-+]+)`)
)

// Components reads dependency files under root. Unpinned or variable versions are skipped, never guessed.
func Components(root string) []Component {
	var out []Component
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		n := d.Name()
		if d.IsDir() {
			if n == ".git" || n == "node_modules" || n == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		add := func(eco, name, ver string) {
			if name != "" && ver != "" {
				out = append(out, Component{eco, name, ver, rel})
			}
		}
		s := string(b)
		switch {
		case n == "go.mod":
			for _, l := range strings.Split(s, "\n") {
				f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "require "))
				if len(f) >= 2 && strings.HasPrefix(f[1], "v") && strings.Contains(f[0], ".") {
					add("Go", f[0], strings.TrimPrefix(f[1], "v"))
				}
			}
		case n == "package-lock.json":
			var lock struct {
				Packages map[string]struct{ Version string }
			}
			if json.Unmarshal(b, &lock) == nil {
				for k, v := range lock.Packages {
					if i := strings.LastIndex(k, "node_modules/"); i >= 0 {
						add("npm", k[i+len("node_modules/"):], v.Version)
					}
				}
			}
		case n == "requirements.txt":
			for _, l := range strings.Split(s, "\n") {
				if m := pyReq.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
					add("PyPI", m[1], m[2])
				}
			}
		case n == "build.gradle" || n == "build.gradle.kts":
			for _, m := range gradleDep.FindAllStringSubmatch(s, -1) {
				add("Maven", m[1]+":"+m[2], m[3])
			}
		case n == "libs.versions.toml":
			vers := map[string]string{}
			inVersions := false
			for _, l := range strings.Split(s, "\n") {
				if t := strings.TrimSpace(l); strings.HasPrefix(t, "[") {
					inVersions = t == "[versions]"
				} else if m := tomlVersion.FindStringSubmatch(l); inVersions && m != nil {
					vers[m[1]] = m[2]
				}
			}
			ver := func(isRef bool, v string) string {
				if isRef {
					return vers[v]
				}
				return v
			}
			for _, m := range tomlLib.FindAllStringSubmatch(s, -1) {
				add("Maven", m[1], ver(m[2] != "", m[3]))
			}
			for _, m := range tomlLib2.FindAllStringSubmatch(s, -1) {
				add("Maven", m[1]+":"+m[2], ver(m[3] != "", m[4]))
			}
		case n == "pom.xml":
			for _, m := range pomDep.FindAllStringSubmatch(s, -1) {
				add("Maven", m[1]+":"+m[2], m[3])
			}
		}
		return nil
	})
	// package.json is only a fallback when there is no lockfile
	if !has(out, "npm") {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || (d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git")) || d.Name() != "package.json" {
				return nil
			}
			var pj struct{ Dependencies, DevDependencies map[string]string }
			b, _ := os.ReadFile(p)
			if json.Unmarshal(b, &pj) == nil {
				rel, _ := filepath.Rel(root, p)
				for _, m := range []map[string]string{pj.Dependencies, pj.DevDependencies} {
					for k, v := range m {
						if v = strings.TrimLeft(v, "^~="); regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
							out = append(out, Component{"npm", k, v, rel})
						}
					}
				}
			}
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Ecosystem+a.Name+a.Version+a.File < b.Ecosystem+b.Name+b.Version+b.File
	})
	return out
}

func has(cs []Component, eco string) bool {
	for _, c := range cs {
		if c.Ecosystem == eco {
			return true
		}
	}
	return false
}
