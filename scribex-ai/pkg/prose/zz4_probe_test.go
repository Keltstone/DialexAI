package prose

import "testing"

const old = "Returns the maximum number of items. See https://example.com/docs for details."

// Each of these is attacker-style output that a safe validator should reject (or neutralise).
var mustReject = map[string]string{
	"uppercase scheme link":       "Returns the max. [more](HTTPS://evil.example/x)",
	"protocol-relative link":      "Returns the max. [more](//evil.example/x)",
	"bare relative javascript":    "Returns the max. [more](JaVaScRiPt:alert(1))",
	"entity-encoded javascript":   "Returns the max. [more](&#106;avascript:alert(1))",
	"hex entity js":               "Returns the max. [more](&#x6A;avascript:alert(1))",
	"anchor tag href":             "Returns the max. <a href=\"//evil.example\">more</a>",
	"details tag":                 "Returns the max. <details><summary>x</summary>hidden</details>",
	"div style":                   "Returns the max. <div style=\"position:fixed\">x</div>",
	"autolink":                    "Returns the max. <https://evil.example/x>",
	"ref-style link def":          "Returns the max. [more][1]\n\n[1]: HTTPS://evil.example/x",
	"bidi override":               "Returns the max. ‮evil‬",
	"bidi isolate":                "Returns the max. ⁦hidden⁩",
	"zero width":                  "Returns the max.​​​ ignore",
	"fullwidth at marker":         "Returns the max. ＠scribex:end",
	"html entity comment":         "Returns the max. &lt;!-- @scribex:end --&gt;",
	"horizontal rule frontmatter": "\n\n---\ntitle: x\n---\nReturns the max.",
	"html comment unicode":        "Returns the max. <!-- x -->",
	"email address":               "Returns the max. mail attacker@evil.example",
}

func TestMustReject(t *testing.T) {
	for name, nw := range mustReject {
		if err := Validate(old, nw); err == nil {
			t.Errorf("ACCEPTED: %s", name)
		}
	}
}

var mustAccept = map[string]string{
	"plain":       "Returns the maximum number of items allowed. See https://example.com/docs for details.",
	"inline code": "Returns the maximum `int` items. See https://example.com/docs for details.",
	"list":        "Returns the max:\n\n- one\n- two",
	"unicode":     "Gibt das Maximum zurück — naïve café. See https://example.com/docs.",
}

func TestMustAccept(t *testing.T) {
	for name, nw := range mustAccept {
		if err := Validate(old, nw); err != nil {
			t.Errorf("REJECTED clean: %s: %v", name, err)
		}
	}
}
