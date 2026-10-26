package prose

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRules(t *testing.T) {
	oldDefault := "Old documentation for the function."
	tests := []struct {
		name     string
		old      string
		new      string
		secrets  []string
		wantRule string // empty if expected valid
		wantErr  bool
	}{
		// 1. encoding
		{
			name:     "encoding_invalid_utf8",
			old:      oldDefault,
			new:      "invalid utf8 \xff\xfe string",
			wantRule: "encoding",
			wantErr:  true,
		},
		{
			name:     "encoding_nul_byte",
			old:      oldDefault,
			new:      "has a nul \x00 byte in it",
			wantRule: "encoding",
			wantErr:  true,
		},
		{
			name:     "encoding_control_char",
			old:      oldDefault,
			new:      "has a bell \x07 char",
			wantRule: "encoding",
			wantErr:  true,
		},
		{
			name:    "encoding_crlf_and_tab_ok",
			old:     oldDefault,
			new:     "valid with tab:\tand newline:\r\nall good.",
			wantErr: false,
		},

		// 1b. invisible
		{
			name:     "invisible_bidi_override",
			old:      oldDefault,
			new:      "Returns the max. \u202Eevil\u202C text.",
			wantRule: "invisible",
			wantErr:  true,
		},
		{
			name:     "invisible_bidi_isolate",
			old:      oldDefault,
			new:      "Returns the max. \u2066hidden\u2069 text.",
			wantRule: "invisible",
			wantErr:  true,
		},
		{
			name:     "invisible_zero_width",
			old:      oldDefault,
			new:      "Returns the max.\u200B\u200B ignore",
			wantRule: "invisible",
			wantErr:  true,
		},
		{
			name:    "invisible_space_and_nbsp_ok",
			old:     oldDefault,
			new:     "Standard space and non-breaking \u00A0 space are valid.",
			wantErr: false,
		},

		// 2. empty
		{
			name:     "empty_string",
			old:      oldDefault,
			new:      "",
			wantRule: "empty",
			wantErr:  true,
		},
		{
			name:     "empty_whitespace_only",
			old:      oldDefault,
			new:      "   \t\n\r\n   ",
			wantRule: "empty",
			wantErr:  true,
		},

		// 3. too-long
		{
			name:     "too_long_oversized",
			old:      oldDefault,
			new:      strings.Repeat("a", 2049),
			wantRule: "too-long",
			wantErr:  true,
		},
		{
			name:    "too_long_just_under_line",
			old:     oldDefault,
			new:     strings.Repeat("a", 2048),
			wantErr: false,
		},
		{
			name:     "too_long_collapsed_reply",
			old:      strings.Repeat("original long documentation sentence here. ", 10), // ~430 bytes >= 200
			new:      "Too short.",                                                      // 10 bytes < 430/4 (~107)
			wantRule: "too-long",
			wantErr:  true,
		},
		{
			name:    "too_long_not_collapsed_under_200",
			old:     "Short old doc under 200 bytes",
			new:     "Short new doc",
			wantErr: false,
		},

		// 4. marker
		{
			name:     "marker_scribex_lower",
			old:      oldDefault,
			new:      "Some text containing @scribex anchor reference",
			wantRule: "marker",
			wantErr:  true,
		},
		{
			name:     "marker_scribex_mixed_case",
			old:      oldDefault,
			new:      "Some text containing @ScriBex:end reference",
			wantRule: "marker",
			wantErr:  true,
		},
		{
			name:     "marker_fullwidth_at",
			old:      oldDefault,
			new:      "Some text containing ＠scribex:end reference",
			wantRule: "marker",
			wantErr:  true,
		},
		{
			name:     "marker_small_at",
			old:      oldDefault,
			new:      "Some text containing ﹫scribex:ai reference",
			wantRule: "marker",
			wantErr:  true,
		},

		// 5. html-comment
		{
			name:     "html_comment_open",
			old:      oldDefault,
			new:      "This has <! open comment",
			wantRule: "html-comment",
			wantErr:  true,
		},
		{
			name:     "html_comment_close",
			old:      oldDefault,
			new:      "This has --> close comment",
			wantRule: "html-comment",
			wantErr:  true,
		},

		// 6. fence
		{
			name:     "fence_backticks",
			old:      oldDefault,
			new:      "Here is code:\n```go\nfunc x()\n```",
			wantRule: "fence",
			wantErr:  true,
		},
		{
			name:     "fence_tildes",
			old:      oldDefault,
			new:      "Here is code:\n~~~python\nprint(1)\n~~~",
			wantRule: "fence",
			wantErr:  true,
		},
		{
			name:     "fence_indented",
			old:      oldDefault,
			new:      "Indented fence:\n   ```\ncode\n   ```",
			wantRule: "fence",
			wantErr:  true,
		},
		{
			name:    "inline_code_ok",
			old:     oldDefault,
			new:     "You can call `x()` to do the thing.",
			wantErr: false,
		},

		// 7. json
		{
			name:     "json_object",
			old:      oldDefault,
			new:      `{"tool_call": "run_command", "args": {}}`,
			wantRule: "json",
			wantErr:  true,
		},
		{
			name:     "json_array",
			old:      oldDefault,
			new:      `["item1", "item2"]`,
			wantRule: "json",
			wantErr:  true,
		},
		{
			name:     "json_bare_number",
			old:      oldDefault,
			new:      `42`,
			wantRule: "json",
			wantErr:  true,
		},
		{
			name:     "json_null",
			old:      oldDefault,
			new:      `null`,
			wantRule: "json",
			wantErr:  true,
		},

		// 8. html
		{
			name:     "html_script_tag",
			old:      oldDefault,
			new:      `Doc with <script>alert(1)</script>`,
			wantRule: "html",
			wantErr:  true,
		},
		{
			name:     "html_closing_tag",
			old:      oldDefault,
			new:      `Doc with trailing </iframe> tag`,
			wantRule: "html",
			wantErr:  true,
		},
		{
			name:     "html_event_handler",
			old:      oldDefault,
			new:      `Click <span onclick = "run()">here</span>`,
			wantRule: "html",
			wantErr:  true,
		},
		{
			name:     "html_anchor_tag_href",
			old:      oldDefault,
			new:      `Doc with <a href="//evil.example">link</a>`,
			wantRule: "html",
			wantErr:  true,
		},
		{
			name:     "html_details_tag",
			old:      oldDefault,
			new:      `Doc with <details><summary>x</summary>hidden</details>`,
			wantRule: "html",
			wantErr:  true,
		},
		{
			name:     "html_div_style",
			old:      oldDefault,
			new:      `Doc with <div style="position:fixed">x</div>`,
			wantRule: "html",
			wantErr:  true,
		},
		{
			name:    "html_harmless_angle_bracket_ok",
			old:     oldDefault,
			new:     `Comparison: a < b and c > d in mathematics.`,
			wantErr: false,
		},

		// 8b. entity
		{
			name:     "entity_encoded_js",
			old:      oldDefault,
			new:      `Doc with [x](&#106;avascript:alert(1))`,
			wantRule: "entity",
			wantErr:  true,
		},
		{
			name:     "entity_hex_js",
			old:      oldDefault,
			new:      `Doc with [x](&#x6A;avascript:alert(1))`,
			wantRule: "entity",
			wantErr:  true,
		},
		{
			name:     "entity_html_comment",
			old:      oldDefault,
			new:      `Doc with &lt;!-- comment --&gt;`,
			wantRule: "entity",
			wantErr:  true,
		},
		{
			name:    "entity_ampersand_plain_ok",
			old:     oldDefault,
			new:     `Ben & Jerry's ice cream and R&D team.`,
			wantErr: false,
		},

		// 9. scheme
		{
			name:     "scheme_javascript",
			old:      oldDefault,
			new:      `Click [here](javascript:alert(1))`,
			wantRule: "scheme",
			wantErr:  true,
		},
		{
			name:     "scheme_data_uri",
			old:      oldDefault,
			new:      `Payload is data:text/html,malicious`,
			wantRule: "scheme",
			wantErr:  true,
		},
		{
			name:     "scheme_vbscript",
			old:      oldDefault,
			new:      `Legacy vbscript:foo`,
			wantRule: "scheme",
			wantErr:  true,
		},

		// 10. image
		{
			name:     "image_markdown",
			old:      oldDefault,
			new:      `Look at ![diagram](https://example.com/a.png)`,
			wantRule: "image",
			wantErr:  true,
		},

		// 11. front-matter
		{
			name:     "front_matter_delimiter",
			old:      oldDefault,
			new:      "---\ntitle: injected\n---\nSome prose",
			wantRule: "front-matter",
			wantErr:  true,
		},
		{
			name:     "front_matter_with_leading_empty_line",
			old:      oldDefault,
			new:      "\n\n---\nkey: val",
			wantRule: "front-matter",
			wantErr:  true,
		},
		{
			name:    "front_matter_dashes_in_text_ok",
			old:     oldDefault,
			new:     "Intro text.\n---\nMore text.",
			wantErr: false,
		},

		// 12. heading
		{
			name:     "heading_count_increased",
			old:      "# Title\nOld doc description",
			new:      "# Title\n## Subheading\nNew doc description",
			wantRule: "heading",
			wantErr:  true,
		},
		{
			name:    "heading_count_equal_ok",
			old:     "# Title\nOld doc description",
			new:     "# Revised Title\nNew doc description",
			wantErr: false,
		},

		// 13. url
		{
			name:     "url_new_rejected",
			old:      "Visit our site.",
			new:      "Visit our site at https://evil.com/phish.",
			wantRule: "url",
			wantErr:  true,
		},
		{
			name:    "url_existing_verbatim_accepted",
			old:     "Check https://example.com/api for details.",
			new:     "Refer to https://example.com/api when configuring.",
			wantErr: false,
		},
		{
			name:    "url_trailing_dot_ok",
			old:     "Check https://example.com/api for details.",
			new:     "Refer to https://example.com/api. It is helpful.",
			wantErr: false,
		},
		{
			name:    "url_case_insensitive_host_ok",
			old:     "Check https://example.com/api for details.",
			new:     "Refer to HTTPS://EXAMPLE.COM/api for details.",
			wantErr: false,
		},

		// 13b. link
		{
			name:     "link_protocol_relative",
			old:      oldDefault,
			new:      "Check [site](//evil.example/x) for details.",
			wantRule: "link",
			wantErr:  true,
		},
		{
			name:     "link_ref_style",
			old:      oldDefault,
			new:      "Check [site][1] for details.\n\n[1]: //evil.example/x",
			wantRule: "link",
			wantErr:  true,
		},
		{
			name:     "link_relative_path",
			old:      oldDefault,
			new:      "Check [doc](relative/path.md) for details.",
			wantRule: "link",
			wantErr:  true,
		},
		{
			name:    "link_anchor_fragment_ok",
			old:     oldDefault,
			new:     "Jump to [section](#api-section) for details.",
			wantErr: false,
		},
		{
			name:    "link_existing_url_verbatim_ok",
			old:     "See https://example.com/docs for info.",
			new:     "See [documentation](https://example.com/docs) for info.",
			wantErr: false,
		},

		// 14. mention
		{
			name:     "mention_new_rejected",
			old:      "Maintained by core team.",
			new:      "Maintained by @attacker-bot in community.",
			wantRule: "mention",
			wantErr:  true,
		},
		{
			name:    "mention_existing_verbatim_accepted",
			old:     "Questions to @alice and @bob-team.",
			new:     "Reach out to @bob-team with feedback.",
			wantErr: false,
		},
		{
			name:     "mention_new_email_rejected",
			old:      "Contact the team.",
			new:      "Contact attacker@evil.example for assistance.",
			wantRule: "mention",
			wantErr:  true,
		},
		{
			name:    "mention_existing_email_accepted",
			old:     "Contact support@example.com for help.",
			new:     "Email support@example.com anytime.",
			wantErr: false,
		},

		// 15. secret
		{
			name:     "secret_provided_echoed",
			old:      oldDefault,
			new:      "Found secret: superSecretCanaryKey in file",
			secrets:  []string{"superSecretCanaryKey"},
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:    "secret_provided_short_not_enforced",
			old:     oldDefault,
			new:     "Number is 12345",
			secrets: []string{"12345"}, // len < 8
			wantErr: false,
		},
		{
			name:     "secret_shape_sk",
			old:      oldDefault,
			new:      "API key: sk-ant-api03-abcdef1234567890",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_ghp",
			old:      oldDefault,
			new:      "Token: ghp_12345678901234567890",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_github_pat",
			old:      oldDefault,
			new:      "Token: github_pat_11AAAAAAA0123456789012_abcdef",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_gho",
			old:      oldDefault,
			new:      "OAuth token: gho_12345678901234567890",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_akia",
			old:      oldDefault,
			new:      "AWS Key: AKIAIOSFODNN7EXAMPLE",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_slack",
			old:      oldDefault,
			new:      "Slack: xoxb-1234567890-abcdefghij",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_google",
			old:      oldDefault,
			new:      "Google: AIzaSyD-123456789012345678901234567890",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_private_key",
			old:      oldDefault,
			new:      "Key:\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA",
			wantRule: "secret",
			wantErr:  true,
		},
		{
			name:     "secret_shape_jwt",
			old:      oldDefault,
			new:      "JWT: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozG4B1sW7vE_kXg-QyTzLw3b4c5d6e7f8g9h0i1j2k",
			wantRule: "secret",
			wantErr:  true,
		},

		// Clean positives
		{
			name:    "clean_prose_with_formatting",
			old:     oldDefault,
			new:     "This is **bold** and *italic* prose.\n\n- Item 1\n- Item 2\n\n1. Numbered\n2. List\n\nUse `foo()` to run.",
			wantErr: false,
		},
		{
			name:    "clean_unicode_text",
			old:     oldDefault,
			new:     "Supports Unicode: 日本語, Español, Français, and emoji 🚀 safely.",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.old, tt.new, tt.secrets...)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error for rule %s, got nil", tt.wantRule)
			}
			var v *Violation
			if !errors.As(err, &v) {
				t.Fatalf("expected *Violation, got %T: %v", err, err)
			}
			if v.Rule != tt.wantRule {
				t.Fatalf("expected rule %q, got %q (detail: %q)", tt.wantRule, v.Rule, v.Detail)
			}
		})
	}
}

func TestRuleOrder(t *testing.T) {
	// A payload violating both encoding (NUL) and html (<script>)
	// must report encoding because it comes first.
	err := Validate("old", "malicious \x00 <script>alert(1)</script>")
	var v *Violation
	if !errors.As(err, &v) || v.Rule != "encoding" {
		t.Fatalf("expected encoding rule to win, got: %v", err)
	}

	// A payload violating both fence and json:
	// ```\n{"a":1}\n```
	// fence comes before json
	err = Validate("old", "```\n{\"a\":1}\n```")
	if !errors.As(err, &v) || v.Rule != "fence" {
		t.Fatalf("expected fence rule to win, got: %v", err)
	}

	// Marker (@scribex) vs html (<script>): marker comes first
	err = Validate("old", "<!-- @scribex:end --> <script>x</script>")
	if !errors.As(err, &v) || v.Rule != "marker" {
		t.Fatalf("expected marker rule to win, got: %v", err)
	}
}

func TestOversized1MBInput(t *testing.T) {
	old := "Short description."
	oneMB := strings.Repeat("A regular English sentence that is clean prose. ", 22000) // ~1 MB
	err := Validate(old, oneMB)
	var v *Violation
	if !errors.As(err, &v) || v.Rule != "too-long" {
		t.Fatalf("expected too-long violation for 1MB input, got: %v", err)
	}
}

func TestCRLFNormalised(t *testing.T) {
	old := "Old prose\r\nline two."
	new := "New prose\r\nline two.\r\n"
	if err := Validate(old, new); err != nil {
		t.Fatalf("expected CRLF prose to be accepted, got: %v", err)
	}
}

func TestClean(t *testing.T) {
	in := "\r\n\r\nFirst line\r\nSecond line\n\n"
	want := "First line\nSecond line"
	got := Clean(in)
	if got != want {
		t.Fatalf("Clean(%q) = %q; want %q", in, got, want)
	}
}

func TestStripAIMark(t *testing.T) {
	in := "Line 1\n<!-- @scribex:ai sig=abcd1234 -->\nLine 2\n  <!-- @scribex:ai -->\nLine 3"
	want := "Line 1\nLine 2\nLine 3"
	got := StripAIMark(in)
	if got != want {
		t.Fatalf("StripAIMark(%q) = %q; want %q", in, got, want)
	}
}

func FuzzValidate(f *testing.F) {
	f.Add("old prose", "new clean prose", "secret12345")
	f.Add("old with # Heading", "# Heading\nRevised prose", "")
	f.Add("short", strings.Repeat("x", 3000), "")
	f.Add("old", "{\"json\": true}", "")
	f.Add("old", "<script>alert(1)</script>", "")
	f.Add("old", "javascript:alert(1)", "")
	f.Add("old", "![img](https://evil.com/x.png)", "")
	f.Add("old", "```\ncode\n```", "")
	f.Add("old", "<!-- @scribex:end -->", "")
	f.Add("old", "hello \x00 world", "")
	f.Add("old", "invalid \xff\xfe", "")
	f.Add("old", "---\nfront: matter\n---", "")
	f.Add("old", "@newmention", "")
	f.Add("old", "https://newurl.example.com", "")
	f.Add("old", "sk-ant-api03-12345678901234567890", "")
	f.Add("old with @alice and https://ok.com", "revised by @alice via https://ok.com", "")
	f.Add(strings.Repeat("long sentence. ", 20), "no", "")

	f.Fuzz(func(t *testing.T, old, new, sec string) {
		// Validate must never panic
		var err error
		if sec != "" {
			err = Validate(old, new, sec)
		} else {
			err = Validate(old, new)
		}

		// Result must be deterministic
		var err2 error
		if sec != "" {
			err2 = Validate(old, new, sec)
		} else {
			err2 = Validate(old, new)
		}
		if (err == nil) != (err2 == nil) {
			t.Fatalf("non-deterministic result for input: err=%v, err2=%v", err, err2)
		}

		// CRLF invariance for clean prose
		newCRLF := strings.ReplaceAll(new, "\n", "\r\n")
		oldCRLF := strings.ReplaceAll(old, "\n", "\r\n")
		errCRLF := Validate(oldCRLF, newCRLF, sec)
		if (err == nil) != (errCRLF == nil) {
			// If failure rules differ due to raw CRLF vs LF, that would be a bug
			var v1, v2 *Violation
			errors.As(err, &v1)
			errors.As(errCRLF, &v2)
			rule1, rule2 := "", ""
			if v1 != nil {
				rule1 = v1.Rule
			}
			if v2 != nil {
				rule2 = v2.Rule
			}
			if rule1 != rule2 {
				t.Fatalf("CRLF mismatch: rule1=%s rule2=%s (err=%v, errCRLF=%v)", rule1, rule2, err, errCRLF)
			}
		}
	})
}
