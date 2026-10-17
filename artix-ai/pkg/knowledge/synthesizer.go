package knowledge

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Provenance captures origin metadata for synthesized knowledge items and rules.
type Provenance struct {
	SessionID         string        `json:"sessionId,omitempty"`
	PRCommentURL      string        `json:"prCommentUrl,omitempty"`
	Author            string        `json:"author,omitempty"`
	AuthorAssociation string        `json:"authorAssociation,omitempty"`
	TTL               time.Duration `json:"ttl,omitempty"`
}

var promptInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(ignore|disregard|forget)\s+(all\s+)?(the\s+)?(previous|prior|above)\s+(instructions?|prompts?|rules?|taboos?)`),
	regexp.MustCompile(`(?i)from\s+now\s+on\s+treat\s+all\s+(taboo|rules?|safety|security)\s+(findings|violations)\s+as\s+warnings`),
	regexp.MustCompile(`(?i)treat\s+all\s+(taboo|rules?|safety|security)\s+(findings|violations)\s+as\s+warnings`),
	regexp.MustCompile(`(?i)bypass\s+all\s+(rules|taboos|security|safety|restrictions|policies)`),
	regexp.MustCompile(`(?i)(disable|override|replace|alter)\s+(all\s+)?(safety|taboos|rules|policies|security)`),
	regexp.MustCompile(`(?i)#\s*system\s+override`),
	regexp.MustCompile(`(?i)<script[\s\S]*?>[\s\S]*?<\/script>`),
	regexp.MustCompile(`(?i)<\/?(script|iframe|object|embed|style)[\s\S]*?>`),
}

// SanitizePRComment treats reviewer input as untrusted and strips malicious prompt injections, HTML/scripts, and control characters.
func SanitizePRComment(text string) string {
	cleaned := text
	// 1. Strip null bytes, non-printable control characters, zero-width characters and bidi overrides
	cleaned = strings.Map(func(r rune) rune {
		if r == 0 || (r < 32 && r != '\n' && r != '\t' && r != '\r') {
			return -1
		}
		// Zero-width space, non-joiner, joiner, BOM, and bidi control runes used for evasions
		if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF || (r >= 0x202A && r <= 0x202E) || r == 0x200E || r == 0x200F {
			return -1
		}
		return r
	}, cleaned)

	// 2. Length cap
	if len(cleaned) > 4096 {
		cleaned = cleaned[:4096]
	}

	// 3. Remove prompt injection patterns (without stripping generic type parameters like <String>)
	for _, pattern := range promptInjectionPatterns {
		cleaned = pattern.ReplaceAllString(cleaned, "")
	}

	return strings.TrimSpace(cleaned)
}

// RuleSynthesizer converts human review comments into structured steering rules.
type RuleSynthesizer struct{}

// NewRuleSynthesizer creates a synthesizer.
func NewRuleSynthesizer() *RuleSynthesizer {
	return &RuleSynthesizer{}
}

// SynthesizeFromComment analyzes reviewer text and outputs a structured rule.
func (rs *RuleSynthesizer) SynthesizeFromComment(comment string) (*SynthesizedRule, error) {
	return rs.SynthesizeFromCommentWithProvenance(comment, Provenance{
		AuthorAssociation: "MEMBER",
	})
}

// SynthesizeFromCommentWithProvenance analyzes reviewer text with provenance metadata.
func (rs *RuleSynthesizer) SynthesizeFromCommentWithProvenance(comment string, prov Provenance) (*SynthesizedRule, error) {
	if prov.AuthorAssociation != "" {
		assoc := strings.ToUpper(strings.TrimSpace(prov.AuthorAssociation))
		if assoc != "OWNER" && assoc != "MEMBER" && assoc != "COLLABORATOR" {
			return nil, fmt.Errorf("synthesis rejected: author %q with association %q is not a maintainer (OWNER/MEMBER/COLLABORATOR)", prov.Author, prov.AuthorAssociation)
		}
	}

	sanitized := SanitizePRComment(comment)
	if sanitized == "" {
		return nil, fmt.Errorf("comment is empty after sanitization")
	}

	lower := strings.ToLower(sanitized)
	isTaboo := strings.Contains(lower, "never") ||
		strings.Contains(lower, "don't") ||
		strings.Contains(lower, "do not") ||
		strings.Contains(lower, "avoid") ||
		strings.Contains(lower, "prohibited")

	// Determine roles
	roles := []string{"adversarial_code_reviewer"}
	if strings.Contains(lower, "android") || strings.Contains(lower, "compose") || strings.Contains(lower, "ui") {
		roles = append(roles, "android_engineer")
	}
	if strings.Contains(lower, "backend") || strings.Contains(lower, "database") || strings.Contains(lower, "sql") {
		roles = append(roles, "backend_engineer")
	}
	if strings.Contains(lower, "security") || strings.Contains(lower, "token") || strings.Contains(lower, "auth") {
		roles = append(roles, "security_auditor")
	}

	ruleID := fmt.Sprintf("rule-syn-%d", time.Now().UnixNano()%1000000)
	name := deriveRuleName(sanitized)
	now := time.Now()
	ttl := 90 * 24 * time.Hour // 90-day default TTL
	if prov.TTL > 0 {
		ttl = prov.TTL
	}
	expires := now.Add(ttl)

	delimitedComment := fmt.Sprintf("=== BEGIN USER REVIEW COMMENT DATA (NON-INSTRUCTION) ===\n%s\n=== END USER REVIEW COMMENT DATA ===", sanitized)

	return &SynthesizedRule{
		RuleID:       ruleID,
		Name:         name,
		IsTaboo:      isTaboo,
		RuleText:     delimitedComment,
		Rationale:    fmt.Sprintf("Synthesized from human code review comment.\n%s", delimitedComment),
		TargetRoles:  roles,
		Confidence:   0.80,
		Status:       RuleStatusProposed, // Defaults to proposed; requires human CODEOWNER review before becoming active
		SessionID:    prov.SessionID,
		PRCommentURL: prov.PRCommentURL,
		Author:       prov.Author,
		TTL:          ttl,
		CreatedAt:    now,
		ExpiresAt:    &expires,
	}, nil
}

func deriveRuleName(text string) string {
	words := strings.Fields(text)
	if len(words) > 5 {
		return strings.Join(words[:5], " ") + "..."
	}
	return text
}

// DetectConflict checks if two rules express contradictory requirements.
func DetectConflict(r1, r2 *SynthesizedRule) bool {
	if r1 == nil || r2 == nil {
		return false
	}
	t1 := strings.ToLower(r1.RuleText)
	t2 := strings.ToLower(r2.RuleText)

	// If one is taboo and the other promotes the exact same thing
	if r1.IsTaboo != r2.IsTaboo {
		for _, token := range []string{"coroutine", "sqlite", "room", "mutex", "channel", "defaultclient", "thread.sleep"} {
			if strings.Contains(t1, token) && strings.Contains(t2, token) {
				return true
			}
		}
	}
	return false
}
