package hunt

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Entry represents an audited attack surface entry point.
type Entry struct {
	Kinds    []string `json:"kinds"` // component, deeplink, route, fileprovider, webview, pending-intent, ios-deeplink, ios-xpc, graphql, grpc
	Name     string   `json:"name"`
	File     string   `json:"file"` // slash path relative to root
	Platform string   `json:"platform,omitempty"`
}

var (
	// Android Manifest components
	manifestActivityRe = regexp.MustCompile(`(?s)<activity\b([^>]*)>`)
	manifestServiceRe  = regexp.MustCompile(`(?s)<service\b([^>]*)>`)
	manifestReceiverRe = regexp.MustCompile(`(?s)<receiver\b([^>]*)>`)
	manifestProviderRe = regexp.MustCompile(`(?s)<provider\b([^>]*)>`)
	nameAttrRe         = regexp.MustCompile(`android:name="([^"]+)"`)
	exportedAttrRe     = regexp.MustCompile(`android:exported="(true|false)"`)
	intentFilterRe     = regexp.MustCompile(`<intent-filter\b`)
	schemeAttrRe       = regexp.MustCompile(`android:scheme="([^"]+)"`)
	fileProviderRe     = regexp.MustCompile(`(?i)(FileProvider|v4\.content\.FileProvider)`)

	// Source code patterns
	deeplinkSourceRe    = regexp.MustCompile(`(?i)(intent\??\.data|getIntent\(\)\.getData\(\)|app:uri=)`)
	webviewBridgeRe     = regexp.MustCompile(`(?i)(addJavascriptInterface|@JavascriptInterface)`)
	pendingIntentRe     = regexp.MustCompile(`(?i)PendingIntent\.(getActivity|getBroadcast|getService|getForegroundService)`)
	navGraphDeeplinkRe  = regexp.MustCompile(`<deepLink\b[^>]*app:uri="([^"]+)"`)

	// Backend patterns
	springRouteRe  = regexp.MustCompile(`(?i)@(GetMapping|PostMapping|PutMapping|DeleteMapping|PatchMapping|RequestMapping)\s*\(`)
	ktorRouteRe    = regexp.MustCompile(`\b(get|post|put|delete|patch)\s*\(\s*["']/`)
	expressRouteRe = regexp.MustCompile(`\b(app|router)\.(get|post|put|delete|patch)\s*\(\s*["']/`)
	pythonRouteRe  = regexp.MustCompile(`(?i)(@app\.route|@router\.(get|post|put|delete)|path\s*\(\s*['"])`)
	goRouteRe      = regexp.MustCompile(`\b(http\.HandleFunc|r\.(GET|POST|PUT|DELETE|HandleFunc)|mux\.HandleFunc)\s*\(\s*["']/`)
	graphqlRe      = regexp.MustCompile(`(?i)(@Query|@Mutation|type\s+(Query|Mutation)\s*\{)`)
	grpcRe         = regexp.MustCompile(`\bRegister[A-Za-z0-9_]+Server\s*\(`)

	// iOS patterns
	iosURLSchemeRe = regexp.MustCompile(`<key>CFBundleURLSchemes</key>\s*<array>(.*?)</array>`)
	iosXPCRe       = regexp.MustCompile(`\b(NSXPCListener|NSXPCInterface)\b`)

	skipDirs = map[string]bool{".git": true, "node_modules": true, "build": true, ".gradle": true, "dist": true}
)

type manifestComponent struct {
	name         string
	isExported   bool
	isFileProv   bool
	hasDeepLinks bool
}

// Entrypoints walks root and lists all discovered entry points.
func Entrypoints(root string) ([]Entry, error) {
	return EntrypointsWithConfig(root, nil)
}

// EntrypointsWithConfig walks root and lists entry points tailored by user configuration.
func EntrypointsWithConfig(root string, cfg *HuntConfig) ([]Entry, error) {
	var srcFiles []string
	var navFiles []string
	var plistFiles []string
	manifestComps := make(map[string]manifestComponent)
	manifestPackage := ""

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		base := d.Name()
		ext := strings.ToLower(filepath.Ext(p))

		switch {
		case base == "AndroidManifest.xml":
			b, err := os.ReadFile(p)
			if err == nil {
				pkgMatch := regexp.MustCompile(`package="([^"]+)"`).FindStringSubmatch(string(b))
				if len(pkgMatch) > 1 {
					manifestPackage = pkgMatch[1]
				}
				comps := parseManifestComponents(string(b))
				for k, v := range comps {
					manifestComps[k] = v
				}
			}
		case ext == ".kt" || ext == ".java" || ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".swift":
			srcFiles = append(srcFiles, p)
		case strings.HasSuffix(p, "navigation") || (ext == ".xml" && strings.Contains(p, "navigation")):
			navFiles = append(navFiles, p)
		case base == "Info.plist" || strings.HasSuffix(base, ".plist"):
			plistFiles = append(plistFiles, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Check Navigation graph deep links
	for _, np := range navFiles {
		b, err := os.ReadFile(np)
		if err == nil && navGraphDeeplinkRe.Match(b) {
			rel, _ := filepath.Rel(root, np)
			srcFiles = append(srcFiles, np)
			_ = rel
		}
	}

	var entries []Entry
	seenEntries := make(map[string]bool)

	for _, p := range srcFiles {
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 1<<21 { // 2MB limit
			continue
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		baseName := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		contentStr := string(b)

		kinds := make(map[string]bool)
		platform := "all"

		// 1. Android Manifest match
		comp, isAndroidComp := matchManifestComponent(baseName, manifestPackage, manifestComps)
		if isAndroidComp {
			platform = "mobile"
			if comp.isExported {
				kinds["component"] = true
			}
			if comp.isFileProv {
				kinds["fileprovider"] = true
			}
			if comp.hasDeepLinks {
				kinds["deeplink"] = true
			}
		}

		// 2. Mobile source heuristics
		if deeplinkSourceRe.Match(b) {
			kinds["deeplink"] = true
			platform = "mobile"
		}
		if webviewBridgeRe.Match(b) {
			kinds["webview"] = true
			platform = "mobile"
		}
		if pendingIntentRe.Match(b) {
			kinds["pending-intent"] = true
			platform = "mobile"
		}

		// 3. Backend route heuristics
		if springRouteRe.Match(b) || ktorRouteRe.Match(b) || expressRouteRe.Match(b) || pythonRouteRe.Match(b) || goRouteRe.Match(b) {
			kinds["route"] = true
			if platform == "all" {
				platform = "backend"
			}
		}
		if graphqlRe.Match(b) {
			kinds["graphql"] = true
			platform = "backend"
		}
		if grpcRe.Match(b) {
			kinds["grpc"] = true
			platform = "backend"
		}

		// 4. iOS heuristics
		if strings.HasSuffix(p, ".swift") {
			if iosXPCRe.Match(b) {
				kinds["ios-xpc"] = true
				platform = "mobile"
			}
			if deeplinkSourceRe.Match(b) {
				kinds["ios-deeplink"] = true
				platform = "mobile"
			}
		}

		// 5. User custom entry points from HuntConfig
		if cfg != nil {
			for _, ce := range cfg.CustomEntrypoints {
				matchedGlob := true
				if ce.FileGlob != "" {
					matchedGlob, _ = filepath.Match(ce.FileGlob, filepath.Base(p))
				}
				if matchedGlob && ce.Regex != "" {
					if re, err := regexp.Compile(ce.Regex); err == nil && re.MatchString(contentStr) {
						kind := ce.Kind
						if kind == "" {
							kind = "custom"
						}
						kinds[kind] = true
						if ce.Platform != "" {
							platform = ce.Platform
						}
					}
				}
			}
		}

		if len(kinds) > 0 {
			var kindList []string
			for k := range kinds {
				kindList = append(kindList, k)
			}
			sort.Strings(kindList)

			key := rel + ":" + baseName
			if !seenEntries[key] {
				seenEntries[key] = true
				entries = append(entries, Entry{
					Kinds:    kindList,
					Name:     baseName,
					File:     rel,
					Platform: platform,
				})
			}
		}
	}

	// 6. iOS Plist URL schemes
	for _, p := range plistFiles {
		b, err := os.ReadFile(p)
		if err == nil && iosURLSchemeRe.Match(b) {
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			baseName := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			entries = append(entries, Entry{
				Kinds:    []string{"ios-deeplink"},
				Name:     baseName,
				File:     rel,
				Platform: "mobile",
			})
		}
	}

	// Filter by Platform if requested
	if cfg != nil && cfg.Platform != "" {
		var filtered []Entry
		for _, e := range entries {
			if e.Platform == "all" || e.Platform == cfg.Platform || (cfg.Platform == "mobile" && (containsKind(e.Kinds, "component") || containsKind(e.Kinds, "deeplink"))) || (cfg.Platform == "backend" && containsKind(e.Kinds, "route")) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Deterministic ordering by (File, Name)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func parseManifestComponents(manifest string) map[string]manifestComponent {
	comps := make(map[string]manifestComponent)
	parseTag := func(re *regexp.Regexp, tagType string) {
		for _, m := range re.FindAllStringSubmatch(manifest, -1) {
			tagContent := m[1]
			nameMatch := nameAttrRe.FindStringSubmatch(tagContent)
			if len(nameMatch) < 2 {
				continue
			}
			name := nameMatch[1]
			expMatch := exportedAttrRe.FindStringSubmatch(tagContent)
			hasFilters := intentFilterRe.MatchString(tagContent)
			hasDeepLinks := schemeAttrRe.MatchString(tagContent)
			isFileProv := tagType == "provider" && fileProviderRe.MatchString(tagContent)

			isExported := false
			if len(expMatch) > 1 {
				isExported = expMatch[1] == "true"
			} else if hasFilters {
				// Android default: components with intent-filters are exported unless explicit false
				isExported = true
			}

			shortName := name[strings.LastIndex(name, ".")+1:]
			comps[shortName] = manifestComponent{
				name:         name,
				isExported:   isExported,
				isFileProv:   isFileProv,
				hasDeepLinks: hasDeepLinks,
			}
			comps[name] = comps[shortName]
		}
	}

	parseTag(manifestActivityRe, "activity")
	parseTag(manifestServiceRe, "service")
	parseTag(manifestReceiverRe, "receiver")
	parseTag(manifestProviderRe, "provider")
	return comps
}

func matchManifestComponent(baseName, pkg string, comps map[string]manifestComponent) (manifestComponent, bool) {
	if c, ok := comps[baseName]; ok {
		return c, true
	}
	if pkg != "" {
		if c, ok := comps[pkg+"."+baseName]; ok {
			return c, true
		}
	}
	// Nested class match: e.g. Outer$Inner -> Inner
	if idx := strings.LastIndex(baseName, "$"); idx >= 0 {
		inner := baseName[idx+1:]
		if c, ok := comps[inner]; ok {
			return c, true
		}
	}
	return manifestComponent{}, false
}

func containsKind(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}

// EntrypointsWithDiff filters discovered entry points to those located in touched files.
func EntrypointsWithDiff(root string, cfg *HuntConfig, changedFiles []string) ([]Entry, error) {
	all, err := EntrypointsWithConfig(root, cfg)
	if err != nil {
		return nil, err
	}
	if len(changedFiles) == 0 {
		return all, nil
	}
	changedMap := make(map[string]bool)
	for _, f := range changedFiles {
		changedMap[filepath.ToSlash(f)] = true
	}
	var filtered []Entry
	for _, ep := range all {
		if changedMap[filepath.ToSlash(ep.File)] {
			filtered = append(filtered, ep)
		}
	}
	return filtered, nil
}
