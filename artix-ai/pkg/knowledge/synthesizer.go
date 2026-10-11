package knowledge

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Provenance captures origin metadata for synthesized knowledge items and rules.
type Provenance struct {
	SessionID    string        `json:"sessionId,omitempty"`
	PRCommentURL string        `json:"prCommentUrl,omitempty"`
	Author       string        `json:"author,omitempty"`
	TTL          time.Duration `json:"ttl,omitempty"`
}

var promptInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(ignore|disregard|forget)\s+(all\s+)?(previous|prior|above)\s+(instructions|prompts|rules|taboos)`),
	regexp.MustCompile(`(?i)bypass\s+all\s+(rules|taboos|security|safety|restrictions|policies)`),
	regexp.MustCompile(`(?i)(disable|override|replace|alter)\s+(all\s+)?(safety|taboos|rules|policies|security)`),
	regexp.MustCompile(`(?i)#\s*system\s+override`),
	regexp.MustCompile(`(?i)<script[\s\S]*?>[\s\S]*?<\/script>`),
	regexp.MustCompile(`(?i)<[\s\S]*?>`),
}

// SanitizePRComment treats reviewer input as untrusted and strips malicious prompt injections, HTML/scripts, and control characters.
func SanitizePRComment(text string) string {
	cleaned := text
	// Remove null bytes and non-printable control characters
	cleaned = strings.Map(func(r rune) rune {
		if r == 0 || (r < 32 && r != '\n' && r != '\t' && r != '\r') {
			return -1
		}
		return r
	}, cleaned)

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
	return rs.SynthesizeFromCommentWithProvenance(comment, Provenance{})
}

// SynthesizeFromCommentWithProvenance analyzes reviewer text with provenance metadata.
func (rs *RuleSynthesizer) SynthesizeFromCommentWithProvenance(comment string, prov Provenance) (*SynthesizedRule, error) {
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

	return &SynthesizedRule{
		RuleID:       ruleID,
		Name:         name,
		IsTaboo:      isTaboo,
		RuleText:     sanitized,
		Rationale:    "Synthesized automatically from human code review comment.",
		TargetRoles:  roles,
		Confidence:   0.80,
		Status:       RuleStatusProposed, // Defaults to proposed; requires CODEOWNER approval before becoming active
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
