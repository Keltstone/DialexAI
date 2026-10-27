package hunt

import (
	"testing"
)

func FuzzTaskflowParser(f *testing.F) {
	// Seed corpus with valid taskflow
	f.Add([]byte(`{"version":1,"name":"test","tasks":[{"name":"t1","model":"classify","prompt":"hello"}]}`))
	f.Add([]byte(`{"version":0}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`invalid json`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic or crash
		_, _ = ParseTaskflow(data)
	})
}

func FuzzTemplateRenderer(f *testing.F) {
	ctx := TemplateContext{
		Globals: map[string]string{"system_rules": "rules"},
		Entry: &Entry{
			Name:  "Test",
			File:  "Test.kt",
			Kinds: []string{"component"},
		},
		Code:    "val x = 1",
		ClassID: "authz",
	}

	f.Add("Entry: {{ entry.name }} Code: {{ entry.code }}")
	f.Add("Script: <script>evil()</script>")
	f.Add("Unresolved: {{ foo.bar.baz }}")
	f.Add("Brackets: {{{ }}} {{ { } }}")

	f.Fuzz(func(t *testing.T, tmpl string) {
		// Must never panic
		_, _ = RenderTemplate(tmpl, ctx)
	})
}
