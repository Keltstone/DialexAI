package doctypes

// builtins returns fresh copies so callers can mutate them.
func builtins() []Type {
	d := func(id, title, aud, grain, out, ground string) Type {
		return Type{ID: id, Title: title, Audience: aud, Granularity: grain, Output: out, Grounding: ground,
			IDPolicy: "none", Language: "en"}
	}
	t := []Type{
		d("tech-reference", "Technical reference", "dev", "file", "docs/reference/{file}.md", "symbol"),
		d("architecture-overview", "Architecture overview", "dev", "module", "docs/architecture/{module}.md", "derived"),
		d("api-reference", "API reference", "dev", "package", "docs/api/{package}.md", "symbol"),
		d("feature-design", "Feature design", "dev", "feature", "docs/features/{feature}/design.md", "derived"),
		d("test-requirements", "Test requirements", "qa", "feature", "docs/features/{feature}/test-requirements.md", "derived"),
		d("requirements", "Requirements", "po", "feature", "docs/features/{feature}/requirements.md", "derived"),
		d("po-feature-summary", "Product feature summary", "po", "feature", "docs/features/{feature}/summary.md", "derived"),
		d("runbook-ops", "Operations runbook", "ops", "repo", "docs/ops/runbook.md", "derived"),
	}
	t[0].Enabled = true
	return t
}

// Skeletons are the required headings for built-in derived/human types (used when Template is empty).
var skeletons = map[string][]string{
	"architecture-overview": {"Purpose", "Components", "Dependencies", "Data flow", "Constraints"},
	"feature-design":        {"Summary", "Behaviour", "Key components", "Edge cases", "Open questions"},
	"test-requirements":     {"Scope", "Existing coverage", "Coverage gaps", "Suggested cases"},
	"requirements":          {"Goal", "Requirements", "Acceptance criteria", "Open questions"},
	"po-feature-summary":    {"What it does", "Who it is for", "Limits", "Open questions"},
	"runbook-ops":           {"Overview", "Configuration", "Failure modes", "Recovery"},
}

// BuiltinSkeleton returns the required headings for a built-in type id (nil if none).
func BuiltinSkeleton(id string) []string { return append([]string(nil), skeletons[id]...) }
