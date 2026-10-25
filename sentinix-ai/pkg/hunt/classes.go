package hunt

import (
	"fmt"
	"regexp"

	"sentinix/pkg/engines"
)

// VulnClass defines a vulnerability category with detection hints and constraints.
type VulnClass struct {
	ID        string           `json:"id"`
	CWE       string           `json:"cwe"`
	Severity  engines.Severity `json:"severity"`
	Hint      string           `json:"hint"`
	Kinds     []string         `json:"kinds"`    // Applicable entrypoint kinds
	Platform  string           `json:"platform"` // "mobile", "backend", "all"
	Disabled  bool             `json:"disabled"`
}

var classIDRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

// baseKindClasses defines the baseline classes audited per entry point kind (matching v1 defaults).
var baseKindClasses = map[string][]string{
	"component":      {"confused-deputy", "intent-redirect", "path-traversal"},
	"deeplink":       {"deeplink-host", "webview-bridge"},
	"route":          {"authz", "injection", "path-traversal"},
	"fileprovider":   {"path-traversal"},
	"webview":        {"webview-bridge"},
	"pending-intent": {"pending-intent-hijack"},
	"graphql":        {"graphql-depth", "authz-bypass", "sql-injection"},
	"grpc":           {"authz-bypass"},
	"ios-deeplink":   {"deeplink-host"},
	"ios-xpc":        {"xpc-authz"},
}

// BuiltinClasses provides metadata, CWEs, and detection prompts for >=15 vulnerability classes.
var BuiltinClasses = map[string]VulnClass{
	"confused-deputy": {
		ID:       "confused-deputy",
		CWE:      "CWE-441",
		Severity: engines.High,
		Kinds:    []string{"component", "deeplink"},
		Platform: "mobile",
	},
	"intent-redirect": {
		ID:       "intent-redirect",
		CWE:      "CWE-601",
		Severity: engines.High,
		Kinds:    []string{"component", "deeplink"},
		Platform: "mobile",
	},
	"path-traversal": {
		ID:       "path-traversal",
		CWE:      "CWE-22",
		Severity: engines.High,
		Kinds:    []string{"component", "fileprovider", "route"},
		Platform: "all",
	},
	"deeplink-host": {
		ID:       "deeplink-host",
		CWE:      "CWE-939",
		Severity: engines.Medium,
		Kinds:    []string{"deeplink", "ios-deeplink"},
		Platform: "mobile",
	},
	"webview-bridge": {
		ID:       "webview-bridge",
		CWE:      "CWE-749",
		Severity: engines.High,
		Kinds:    []string{"webview", "component", "deeplink"},
		Platform: "mobile",
	},
	"tapjacking": {
		ID:       "tapjacking",
		CWE:      "CWE-1021",
		Severity: engines.Medium,
		Hint:     "Check if this exported sensitive UI activity or view has filterTouchesWhenObscured enabled to prevent tapjacking/overlay attacks.",
		Kinds:    []string{"component"},
		Platform: "mobile",
	},
	"pending-intent-hijack": {
		ID:       "pending-intent-hijack",
		CWE:      "CWE-927",
		Severity: engines.High,
		Hint:     "Look for mutable PendingIntents without FLAG_IMMUTABLE and explicit component, allowing malicious apps to mutate intent extras or action.",
		Kinds:    []string{"component", "pending-intent"},
		Platform: "mobile",
	},
	"sql-injection": {
		ID:       "sql-injection",
		CWE:      "CWE-89",
		Severity: engines.Critical,
		Hint:     "Check if user-controlled parameters are concatenated directly into raw SQL queries, ContentProvider queries, or ORM raw strings.",
		Kinds:    []string{"route", "component", "graphql"},
		Platform: "all",
	},
	"injection": { // Alias matching v1 class vocabulary
		ID:       "injection",
		CWE:      "CWE-89",
		Severity: engines.Critical,
		Kinds:    []string{"route"},
		Platform: "backend",
	},
	"authz": { // Alias matching v1 class vocabulary
		ID:       "authz",
		CWE:      "CWE-862",
		Severity: engines.High,
		Kinds:    []string{"route"},
		Platform: "backend",
	},
	"command-injection": {
		ID:       "command-injection",
		CWE:      "CWE-78",
		Severity: engines.Critical,
		Hint:     "Look for unescaped user input passed to Runtime.getRuntime().exec, ProcessBuilder, sh -c, or shell utilities.",
		Kinds:    []string{"route", "component"},
		Platform: "all",
	},
	"authz-bypass": {
		ID:       "authz-bypass",
		CWE:      "CWE-862",
		Severity: engines.High,
		Hint:     "Verify if this endpoint or component enforces authentication and role-based permissions before processing requests.",
		Kinds:    []string{"route", "component", "graphql", "grpc"},
		Platform: "all",
	},
	"ssrf": {
		ID:       "ssrf",
		CWE:      "CWE-918",
		Severity: engines.High,
		Hint:     "Check if an incoming URL parameter is fetched by the server without restricting protocols (http/https only) and private IP ranges (127.0.0.1, 169.254.169.254, RFC1918).",
		Kinds:    []string{"route", "deeplink"},
		Platform: "all",
	},
	"insecure-deserialization": {
		ID:       "insecure-deserialization",
		CWE:      "CWE-502",
		Severity: engines.Critical,
		Hint:     "Look for deserialization of untrusted byte streams using Java ObjectInputStream, Python pickle, PHP unserialize, or unsafe YAML/JSON loaders.",
		Kinds:    []string{"route", "component"},
		Platform: "all",
	},
	"xss-reflected": {
		ID:       "xss-reflected",
		CWE:      "CWE-79",
		Severity: engines.Medium,
		Hint:     "Check if user parameters are reflected into HTML, scripts, or WebViews without context-aware HTML encoding.",
		Kinds:    []string{"route", "webview"},
		Platform: "all",
	},
	"idor": {
		ID:       "idor",
		CWE:      "CWE-639",
		Severity: engines.High,
		Hint:     "Verify that resource IDs (user ID, account number, document UUID) are validated against the authenticated session/tenant.",
		Kinds:    []string{"route", "graphql"},
		Platform: "backend",
	},
	"hardcoded-secret": {
		ID:       "hardcoded-secret",
		CWE:      "CWE-798",
		Severity: engines.High,
		Hint:     "Check for hardcoded API keys, private keys, JWT signing secrets, or credentials embedded directly in source code.",
		Kinds:    []string{"component", "route", "ios-deeplink"},
		Platform: "all",
	},
	"xpc-authz": {
		ID:       "xpc-authz",
		CWE:      "CWE-285",
		Severity: engines.High,
		Hint:     "Verify if this iOS/macOS XPC listener validates connecting process code signing requirements and audit tokens.",
		Kinds:    []string{"ios-xpc"},
		Platform: "mobile",
	},
	"graphql-depth": {
		ID:       "graphql-depth",
		CWE:      "CWE-776",
		Severity: engines.Medium,
		Hint:     "Look for nested recursive GraphQL resolvers without depth limiting or query complexity calculation.",
		Kinds:    []string{"graphql"},
		Platform: "backend",
	},
}

// ClassLibrary manages active vulnerability classes, combining built-ins, packs, and project overrides.
type ClassLibrary struct {
	classes map[string]VulnClass
	custom  map[string][]string // kind -> custom class IDs
}

// NewClassLibrary initializes the class library with built-ins and custom overrides.
func NewClassLibrary(custom []ClassConfig, packs []*Pack) (*ClassLibrary, error) {
	lib := &ClassLibrary{
		classes: make(map[string]VulnClass),
		custom:  make(map[string][]string),
	}
	// Copy built-ins
	for k, v := range BuiltinClasses {
		lib.classes[k] = v
	}

	// Apply pack classes
	for _, p := range packs {
		if p == nil {
			continue
		}
		for _, cs := range p.Classes {
			for _, c := range cs {
				existing := lib.classes[c.ID]
				existing.ID = c.ID
				if c.CWE != "" {
					existing.CWE = c.CWE
				}
				if c.Hint != "" {
					existing.Hint = c.Hint
				}
				lib.classes[c.ID] = existing
			}
		}
	}

	// Apply user custom classes
	for _, cc := range custom {
		if !classIDRegex.MatchString(cc.ID) {
			return nil, fmt.Errorf("invalid class ID %q in config", cc.ID)
		}
		sev := engines.High
		if cc.MaxSev != "" {
			if s, err := engines.ParseSeverity(cc.MaxSev); err == nil {
				sev = s
			}
		}
		lib.classes[cc.ID] = VulnClass{
			ID:       cc.ID,
			CWE:      cc.CWE,
			Hint:     cc.Hint,
			Kinds:    cc.Kinds,
			Severity: sev,
			Disabled: cc.Disabled,
			Platform: "all",
		}
		for _, k := range cc.Kinds {
			lib.custom[k] = append(lib.custom[k], cc.ID)
		}
	}

	return lib, nil
}

// ClassesFor returns the active, enabled class IDs applicable to the entry point and platform filter.
// Preserves base kind classes, custom classes, and signed pack additions in order.
func (lib *ClassLibrary) ClassesFor(ep Entry, platform string, packs []*Pack) []string {
	var result []string
	seen := make(map[string]bool)

	add := func(c string) {
		if !seen[c] {
			if vc, ok := lib.classes[c]; ok && vc.Disabled {
				return
			}
			seen[c] = true
			result = append(result, c)
		}
	}

	for _, k := range ep.Kinds {
		// Kind platform check: mobile vs backend
		if platform != "" && kindPlatform[k] != "" && kindPlatform[k] != platform {
			continue
		}

		// 1. Base kind classes
		for _, c := range baseKindClasses[k] {
			add(c)
		}

		// 2. Custom classes for this kind
		for _, c := range lib.custom[k] {
			add(c)
		}

		// 3. Signed pack classes for this kind (appended at the end)
		for _, p := range packs {
			for _, c := range p.Classes[k] {
				add(c.ID)
			}
		}
	}

	return result
}

// GetClass looks up class metadata by ID.
func (lib *ClassLibrary) GetClass(id string) (VulnClass, bool) {
	c, ok := lib.classes[id]
	return c, ok
}
