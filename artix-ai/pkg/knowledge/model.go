package knowledge

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// KnowledgeCategory classifies institutional learnings.
type KnowledgeCategory string

const (
	CategoryArchitecture  KnowledgeCategory = "architecture"
	CategoryDebugging     KnowledgeCategory = "debugging"
	CategoryFlakyTest     KnowledgeCategory = "flaky_test"
	CategoryBuildManifest KnowledgeCategory = "build_manifest"
)

// CurrentSchemaVersion is the latest schema version for KnowledgeItem.
const CurrentSchemaVersion = 2

// ItemStatus represents the governance and lifecycle state of a KnowledgeItem.
type ItemStatus string

const (
	StatusProposed   ItemStatus = "proposed"
	StatusActive     ItemStatus = "active"
	StatusApproved   ItemStatus = "approved"
	StatusDemoted    ItemStatus = "demoted"
	StatusDeprecated ItemStatus = "deprecated"
)

// KnowledgeItem encapsulates an institutional learning distilled from an engineering run.
type KnowledgeItem struct {
	SchemaVersion   int               `json:"schemaVersion"`
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Category        KnowledgeCategory `json:"category"`
	Context         string            `json:"context"`
	Breakthrough    string            `json:"breakthrough"`
	AssociatedRules []string          `json:"associatedRules,omitempty"`
	Author          string            `json:"author,omitempty"`
	SourcePR        string            `json:"sourcePr,omitempty"`
	SessionID       string            `json:"sessionId,omitempty"`
	SessionCount    int               `json:"sessionCount,omitempty"`
	PRCommentURL    string            `json:"prCommentUrl,omitempty"`
	ModelID         string            `json:"modelId,omitempty"`
	Confidence      float64           `json:"confidence"`
	ContentHash     string            `json:"contentHash,omitempty"`
	Status          string            `json:"status,omitempty"` // "proposed", "active", "approved", "demoted", "deprecated"
	ApprovedBy      string            `json:"approvedBy,omitempty"`
	DemotedReason   string            `json:"demotedReason,omitempty"`
	RatifiedAt      *time.Time        `json:"ratifiedAt,omitempty"`
	LastEvaluatedAt *time.Time        `json:"lastEvaluatedAt,omitempty"`
	LastEvalResult  *ABEvalResult     `json:"lastEvalResult,omitempty"`
	TTL             time.Duration     `json:"ttl,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	ExpiresAt       *time.Time        `json:"expiresAt,omitempty"`
}

// IsExpired checks if a knowledge item has exceeded its TTL.
func (ki *KnowledgeItem) IsExpired() bool {
	return ki.ExpiresAt != nil && time.Now().After(*ki.ExpiresAt)
}

// IsActive returns true if the knowledge item is ratified/approved, not expired, and not demoted.
func (ki *KnowledgeItem) IsActive() bool {
	st := strings.ToLower(strings.TrimSpace(ki.Status))
	if st != "active" && st != "approved" {
		return false
	}
	if ki.IsExpired() {
		return false
	}
	return true
}

// Ratify promotes a proposed knowledge item to active status upon human CODEOWNER ratification.
func (ki *KnowledgeItem) Ratify(approver string, codeowners ...[]string) error {
	trimmed := strings.TrimSpace(approver)
	if trimmed == "" {
		return errors.New("cannot ratify knowledge item: approver identity is required")
	}
	lower := strings.ToLower(trimmed)
	if lower == "artix-agent" || lower == "artix-bot" || lower == "bot" || strings.Contains(lower, "[bot]") {
		return fmt.Errorf("bot identity %q cannot ratify knowledge items; human CODEOWNER approval required", trimmed)
	}
	if len(codeowners) > 0 && len(codeowners[0]) > 0 {
		isOwner := false
		for _, owner := range codeowners[0] {
			if strings.EqualFold(strings.TrimSpace(owner), trimmed) {
				isOwner = true
				break
			}
		}
		if !isOwner {
			return fmt.Errorf("approver %q is not authorized in CODEOWNERS list: %v", trimmed, codeowners[0])
		}
	}
	now := time.Now()
	ki.Status = "active"
	ki.ApprovedBy = trimmed
	ki.RatifiedAt = &now
	return nil
}

// AutoDemote changes the item status to demoted due to A/B regression or policy violation.
func (ki *KnowledgeItem) AutoDemote(reason string) {
	ki.Status = "demoted"
	ki.DemotedReason = strings.TrimSpace(reason)
}

// RuleStatus represents the governance state of a synthesized rule.
type RuleStatus string

const (
	RuleStatusProposed RuleStatus = "proposed"
	RuleStatusActive   RuleStatus = "active"
	RuleStatusApproved RuleStatus = "approved"
	RuleStatusDemoted  RuleStatus = "demoted"
	RuleStatusRejected RuleStatus = "rejected"
)

// SynthesizedRule represents a steering invariant derived from human review or agent convergence.
type SynthesizedRule struct {
	RuleID        string        `json:"ruleId"`
	Name          string        `json:"name"`
	IsTaboo       bool          `json:"isTaboo"` // true = Taboo Space forbidden argument; false = Heuristic rule
	RuleText      string        `json:"ruleText"`
	Rationale     string        `json:"rationale"`
	TargetRoles   []string      `json:"targetRoles"`
	SourcePR      string        `json:"sourcePr,omitempty"`
	SessionID     string        `json:"sessionId,omitempty"`
	PRCommentURL  string        `json:"prCommentUrl,omitempty"`
	Author        string        `json:"author,omitempty"`
	Confidence    float64       `json:"confidence"`
	Status        RuleStatus    `json:"status"` // "proposed", "active", "approved", "demoted", "rejected"
	ApprovedBy    string        `json:"approvedBy,omitempty"`
	DemotedReason string        `json:"demotedReason,omitempty"`
	RatifiedAt    *time.Time    `json:"ratifiedAt,omitempty"`
	TTL           time.Duration `json:"ttl,omitempty"`
	CreatedAt     time.Time     `json:"createdAt"`
	ExpiresAt     *time.Time    `json:"expiresAt,omitempty"`
}

// Approve promotes a proposed rule to approved status. Enforces that the approver is human (not a bot).
func (r *SynthesizedRule) Approve(approver string, codeowners ...[]string) error {
	trimmed := strings.TrimSpace(approver)
	if trimmed == "" {
		return errors.New("cannot approve rule: approver identity is required")
	}
	lower := strings.ToLower(trimmed)
	if lower == "artix-agent" || lower == "artix-bot" || lower == "bot" || strings.Contains(lower, "[bot]") {
		return fmt.Errorf("bot identity %q cannot approve synthesized rules; human approval required", trimmed)
	}
	if len(codeowners) > 0 && len(codeowners[0]) > 0 {
		isOwner := false
		for _, owner := range codeowners[0] {
			if strings.EqualFold(strings.TrimSpace(owner), trimmed) {
				isOwner = true
				break
			}
		}
		if !isOwner {
			return fmt.Errorf("approver %q is not authorized in CODEOWNERS list: %v", trimmed, codeowners[0])
		}
	}
	now := time.Now()
	r.Status = RuleStatusApproved
	r.ApprovedBy = trimmed
	r.RatifiedAt = &now
	return nil
}

// Ratify promotes a proposed rule to active status upon human CODEOWNER ratification.
func (r *SynthesizedRule) Ratify(approver string, codeowners ...[]string) error {
	trimmed := strings.TrimSpace(approver)
	if trimmed == "" {
		return errors.New("cannot ratify rule: approver identity is required")
	}
	lower := strings.ToLower(trimmed)
	if lower == "artix-agent" || lower == "artix-bot" || lower == "bot" || strings.Contains(lower, "[bot]") {
		return fmt.Errorf("bot identity %q cannot ratify synthesized rules; human CODEOWNER approval required", trimmed)
	}
	if len(codeowners) > 0 && len(codeowners[0]) > 0 {
		isOwner := false
		for _, owner := range codeowners[0] {
			if strings.EqualFold(strings.TrimSpace(owner), trimmed) {
				isOwner = true
				break
			}
		}
		if !isOwner {
			return fmt.Errorf("approver %q is not authorized in CODEOWNERS list: %v", trimmed, codeowners[0])
		}
	}
	now := time.Now()
	r.Status = RuleStatusActive
	r.ApprovedBy = trimmed
	r.RatifiedAt = &now
	return nil
}

// AutoDemote marks the synthesized rule as demoted.
func (r *SynthesizedRule) AutoDemote(reason string) {
	r.Status = RuleStatusDemoted
	r.DemotedReason = strings.TrimSpace(reason)
}

// IsActive returns true if the rule is approved/active, not expired, and not demoted.
func (r *SynthesizedRule) IsActive() bool {
	if r.Status != RuleStatusApproved && r.Status != RuleStatusActive {
		return false
	}
	if r.ExpiresAt != nil && time.Now().After(*r.ExpiresAt) {
		return false
	}
	return true
}
