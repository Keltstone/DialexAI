package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"artix/pkg/policy"
)

// CouncilMemberProvenance records an individual stakeholder council participant.
type CouncilMemberProvenance struct {
	Role      string `json:"role"`
	PersonaID string `json:"personaId"`
	Name      string `json:"name"`
}

// SteeringRuleProvenance records an active steering rule and its content digest.
type SteeringRuleProvenance struct {
	ID      string `json:"id"`
	RelPath string `json:"relPath"`
	Hash    string `json:"hash"`
}

// StoryProvenance provides a machine-readable governance and audit trail for a StorySpec.
type StoryProvenance struct {
	SchemaVersion           int                        `json:"schema_version"`
	SpecID                  string                     `json:"specId"`
	Title                   string                     `json:"title"`
	CreatedAt               time.Time                  `json:"createdAt"`
	GeneratingUser          string                     `json:"generatingUser"`
	DeliberationMethod      string                     `json:"deliberationMethod"` // "multi_persona_deliberation" or "deterministic_template"
	DeliberationRounds      int                        `json:"deliberationRounds"`
	ModelProvider           string                     `json:"modelProvider,omitempty"`
	ModelName               string                     `json:"modelName,omitempty"`
	Style                   StyleVector                `json:"style,omitempty"`
	EnterpriseMode          bool                       `json:"enterpriseMode"`
	CouncilMembers          []CouncilMemberProvenance  `json:"councilMembers"`
	ActiveSteeringHashes    []string                   `json:"activeSteeringHashes"`
	AcceptanceCriteriaCount int                        `json:"acceptanceCriteriaCount"`
	DecisionsCount          int                        `json:"decisionsCount"`
	DeliberationTranscript  []DeliberationRound        `json:"deliberationTranscript,omitempty"`
}

// BuildStoryProvenance constructs a full provenance sidecar record for a StorySpec.
func BuildStoryProvenance(spec *StorySpec, pCtx *PlanningContext, modelProvider, modelName string, members []CouncilMember) *StoryProvenance {
	username := os.Getenv("USER")
	if username == "" {
		if u, err := user.Current(); err == nil {
			username = u.Username
		}
	}
	if username == "" {
		username = "unknown"
	}

	enterpriseMode := policy.IsEnterprise()

	method := "deterministic_template"
	roundCount := len(spec.DeliberationRounds)
	if modelProvider != "" || modelName != "" {
		method = "multi_persona_deliberation"
	}

	var memberProv []CouncilMemberProvenance
	for _, m := range members {
		memberProv = append(memberProv, CouncilMemberProvenance{
			Role:      m.Role,
			PersonaID: m.Persona.ID,
			Name:      m.Persona.Name,
		})
	}

	var steeringHashes []string
	if pCtx != nil {
		for _, s := range pCtx.Steering {
			for _, r := range s.BoundRules {
				hash := r.Hash
				if hash == "" && r.Content != "" {
					h := sha256.Sum256([]byte(r.Content))
					hash = "sha256:" + hex.EncodeToString(h[:])
				}
				steeringHashes = append(steeringHashes, hash)
			}
		}
	}

	return &StoryProvenance{
		SchemaVersion:           1,
		SpecID:                  spec.ID,
		Title:                   spec.Title,
		CreatedAt:               time.Now().UTC(),
		GeneratingUser:          username,
		DeliberationMethod:      method,
		DeliberationRounds:      roundCount,
		ModelProvider:           modelProvider,
		ModelName:               modelName,
		Style:                   spec.Style,
		EnterpriseMode:          enterpriseMode,
		CouncilMembers:          memberProv,
		ActiveSteeringHashes:    steeringHashes,
		AcceptanceCriteriaCount: len(spec.AcceptanceCriteria),
		DecisionsCount:          len(spec.Decisions),
		DeliberationTranscript:  spec.DeliberationRounds,
	}
}

// WriteSpecWithProvenance writes both the StorySpec markdown and its provenance JSON sidecar.
func WriteSpecWithProvenance(specsDir string, spec *StorySpec, prov *StoryProvenance) (string, string, error) {
	if spec == nil || spec.ID == "" {
		return "", "", fmt.Errorf("invalid spec: missing ID")
	}

	if err := os.MkdirAll(specsDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create specs directory: %w", err)
	}

	if prov == nil {
		prov = &StoryProvenance{
			SchemaVersion: 1,
			SpecID:        spec.ID,
			Title:         spec.Title,
		}
	}

	if prov.CreatedAt.IsZero() {
		prov.CreatedAt = time.Now().UTC()
	}
	if prov.AcceptanceCriteriaCount == 0 {
		prov.AcceptanceCriteriaCount = len(spec.AcceptanceCriteria)
	}
	if prov.DecisionsCount == 0 {
		prov.DecisionsCount = len(spec.Decisions)
	}
	if prov.DeliberationRounds == 0 && len(spec.DeliberationRounds) > 0 {
		prov.DeliberationRounds = len(spec.DeliberationRounds)
	}

	specPath := filepath.Join(specsDir, fmt.Sprintf("%s.md", spec.ID))
	rawMd := spec.RawMarkdown
	if rawMd == "" {
		rawMd = FormatToMarkdown(spec)
	}
	if err := os.WriteFile(specPath, []byte(rawMd), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write spec markdown file: %w", err)
	}

	provPath := filepath.Join(specsDir, fmt.Sprintf("%s.provenance.json", spec.ID))
	data, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return specPath, "", fmt.Errorf("failed to marshal provenance: %w", err)
	}
	if err := os.WriteFile(provPath, data, 0644); err != nil {
		return specPath, "", fmt.Errorf("failed to write provenance JSON file: %w", err)
	}

	return specPath, provPath, nil
}

// LoadStoryProvenance reads an existing story provenance JSON sidecar.
func LoadStoryProvenance(specsDir, specID string) (*StoryProvenance, error) {
	provPath := filepath.Join(specsDir, fmt.Sprintf("%s.provenance.json", specID))
	data, err := os.ReadFile(provPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read story provenance: %w", err)
	}

	var prov StoryProvenance
	if err := json.Unmarshal(data, &prov); err != nil {
		return nil, fmt.Errorf("failed to parse story provenance: %w", err)
	}

	return &prov, nil
}
