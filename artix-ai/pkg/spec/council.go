package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"artix/pkg/persona"
	"artix/pkg/repo"
	"socratix/pkg/model"
	"socratix/pkg/runner"
)

// Deliberator is a function that asks a council member persona for input during deliberation rounds.
type Deliberator func(ctx context.Context, role string, persona model.Persona, systemInstructions, prompt string) (string, error)

// CouncilMember represents an assembled participant in the planning council.
type CouncilMember struct {
	Role    string        `json:"role"`
	Persona model.Persona `json:"persona"`
}

// Council assembles stakeholder roles to deliberate and produce a StorySpec.
// It supports model-backed multi-round dialectic deliberation when a Deliberator or Runner is attached,
// with a deterministic structured template fallback.
// CouncilStreamEvent represents a real-time event during council deliberation.
type CouncilStreamEvent struct {
	Type        string         `json:"type"`
	Timestamp   time.Time      `json:"timestamp"`
	SpecID      string         `json:"specId,omitempty"`
	Round       int            `json:"round,omitempty"`
	TotalRounds int            `json:"totalRounds,omitempty"`
	Role        string         `json:"role,omitempty"`
	PersonaID   string         `json:"personaId,omitempty"`
	Status      string         `json:"status,omitempty"`
	Message     string         `json:"message,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// CouncilStreamHandler is a callback for real-time council stream events.
type CouncilStreamHandler func(event CouncilStreamEvent)

type Council struct {
	members       []CouncilMember
	registry      *persona.Registry
	deliberator   Deliberator
	runner        runner.AgentRunner
	agent         model.Agent
	streamHandler CouncilStreamHandler
}

// SetStreamHandler registers a stream handler callback for real-time council events.
func (c *Council) SetStreamHandler(h CouncilStreamHandler) {
	c.streamHandler = h
}

// NewCouncil initializes a Stakeholder Council with the standard stakeholder team.
func NewCouncil(registry *persona.Registry) *Council {
	c := &Council{
		registry: registry,
		members:  make([]CouncilMember, 0),
	}

	roles := []struct {
		role string
		id   string
	}{
		{"Product Owner", "product_owner_lead"},
		{"Senior Architect", "senior_software_architect"},
		{"QA Lead", "qa_testing_lead"},
		{"Engineering Manager", "engineering_manager"},
	}

	for _, r := range roles {
		if p, ok := registry.Get(r.id); ok {
			c.members = append(c.members, CouncilMember{
				Role:    r.role,
				Persona: p,
			})
		}
	}

	return c
}

// SetDeliberator configures a custom model deliberation function.
func (c *Council) SetDeliberator(d Deliberator) {
	c.deliberator = d
}

// SetRunner adapts an AgentRunner from socratix to drive the council deliberation.
func (c *Council) SetRunner(r runner.AgentRunner, agent model.Agent) {
	c.runner = r
	c.agent = agent
	c.deliberator = func(ctx context.Context, role string, p model.Persona, sysInstructions, prompt string) (string, error) {
		topic := fmt.Sprintf("Stakeholder Council Deliberation (%s)", role)
		res, err := r.Respond(ctx, agent, topic, prompt, sysInstructions, nil, "")
		if err != nil {
			return "", err
		}
		return res.Content, nil
	}
}

// Members returns the assembled council members.
func (c *Council) Members() []CouncilMember {
	return c.members
}

func (c *Council) getMemberPersona(roleID string) model.Persona {
	for _, m := range c.members {
		if m.Persona.ID == roleID {
			return m.Persona
		}
	}
	if c.registry != nil {
		if p, ok := c.registry.Get(roleID); ok {
			return p
		}
	}
	return model.Persona{
		ID:   roleID,
		Name: strings.ReplaceAll(roleID, "_", " "),
		Role: "Council Participant",
	}
}

func (c *Council) getSynthesizerPersona() model.Persona {
	if c.registry != nil {
		if p, ok := c.registry.Get("lead_council_synthesizer"); ok {
			return p
		}
	}
	return model.Persona{
		ID:   "lead_council_synthesizer",
		Name: "Lead Council Synthesizer",
		Role: "Neutral Requirements & Architecture Synthesizer",
	}
}

// DeliberationDraft represents the structured synthesis from model deliberation rounds.
type DeliberationDraft struct {
	Title              string                 `json:"title"`
	UserStory          string                 `json:"userStory"`
	InScope            []string               `json:"inScope"`
	OutOfScope         []string               `json:"outOfScope"`
	AcceptanceCriteria []Scenario             `json:"acceptanceCriteria"`
	Decisions          []ArchitectureDecision `json:"decisions"`
	FileManifest       []FileMutation         `json:"fileManifest"`
	TestCommands       []string               `json:"testCommands"`
}

// Plan coordinates requirements planning. If a deliberator or runner is attached, it runs
// structured deliberation rounds across the council personas; otherwise it produces a structured draft.
func (c *Council) Plan(ctx context.Context, pCtx *PlanningContext) (*StorySpec, error) {
	if pCtx == nil || strings.TrimSpace(pCtx.StoryPrompt) == "" {
		return nil, fmt.Errorf("story prompt cannot be empty")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	specID := fmt.Sprintf("STORY-%d", nextSpecNumber())
	title := deriveTitle(pCtx.StoryPrompt)

	// If model-backed deliberation is enabled, run the multi-round deliberation
	if c.deliberator != nil {
		spec, err := c.deliberate(ctx, pCtx, specID, title)
		if err == nil && spec != nil {
			return spec, nil
		}
		// On deliberation error, fall through to deterministic draft with warning if needed or return err
	}

	// Deterministic Structured Template Path
	spec := &StorySpec{
		ID:        specID,
		Title:     title,
		UserStory: fmt.Sprintf("As a developer/user, I want to %s, so that system functionality is enhanced.", pCtx.StoryPrompt),
		Style:     pCtx.Style,
		InScope: []string{
			pCtx.StoryPrompt,
			"Unit test verification suite covering edge cases",
		},
		OutOfScope: []string{
			"Unrelated refactors outside target domain",
			"Breaking public API contracts without deprecation window",
		},
		AcceptanceCriteria: []Scenario{
			{
				Name:  "Standard Success Path",
				Given: "the system is in a valid running state",
				When:  fmt.Sprintf("the user initiates: %s", pCtx.StoryPrompt),
				Then:  "the operation succeeds with expected state transitions and returns status OK",
			},
			{
				Name:  "Adversarial Error & Timeout Handling",
				Given: "downstream dependencies experience transient failure or network timeout",
				When:  "the operation is triggered",
				Then:  "the system fails safely without leaking unhandled exceptions or state corruption",
			},
		},
		Decisions: []ArchitectureDecision{
			{
				ID:       "ADR-01",
				Title:    "Layer Isolation & Boundary Protection",
				Context:  "Implementing feature while respecting project architectural boundaries.",
				Decision: "Encapsulate logic within domain repository layer; expose clean interface abstractions.",
				Consequences: []string{
					"Prevents direct database coupling in UI presentation layer",
					"Ensures independent testability with mock drivers",
				},
			},
		},
		FileManifest: make([]FileMutation, 0),
		TestCommands: make([]string, 0),
		DeliberationRounds: []DeliberationRound{
			{
				Round:      1,
				Role:       "Deterministic Scaffold",
				Topic:      "Template Generation",
				Transcript: "Council executed in deterministic template mode (no model runner configured). Pass --provider and --model to activate live multi-persona AI deliberation.",
			},
		},
	}

	// Tailor test commands based on detected repo ecosystem
	c.applyRepoTestCommands(spec, pCtx.RepoContext)

	// Apply Style Vectors
	switch pCtx.Style {
	case StylePonytail:
		spec.UserStory = fmt.Sprintf("Executive Summary: Strategic implementation of %q to optimize delivery velocity and user value.", pCtx.StoryPrompt)
	case StyleCaveman:
		spec.UserStory = fmt.Sprintf("Build %q. Write tests. Ensure zero regressions.", pCtx.StoryPrompt)
	}

	spec.RawMarkdown = FormatToMarkdown(spec)

	if c.streamHandler != nil {
		c.streamHandler(CouncilStreamEvent{
			Type:        "council_done",
			Timestamp:   time.Now().UTC(),
			SpecID:      specID,
			TotalRounds: 1,
			Status:      "completed",
			Message:     fmt.Sprintf("Council template generation completed for %s", specID),
		})
	}
	return spec, nil
}

// deliberate executes 3 structured rounds among assembled council members:
// Round 1: Role Proposals & Framing (PO, Architect, QA, EM)
// Round 2: Adversarial Critique & Conflict Resolution
// Round 3: Synthesis into Canonical StorySpec
func (c *Council) deliberate(ctx context.Context, pCtx *PlanningContext, specID, defaultTitle string) (*StorySpec, error) {
	if c.streamHandler != nil {
		c.streamHandler(CouncilStreamEvent{
			Type:        "council_start",
			Timestamp:   time.Now().UTC(),
			SpecID:      specID,
			TotalRounds: 3,
			Message:     fmt.Sprintf("Starting 3-round Stakeholder Council deliberation for spec %s", specID),
		})
	}

	type roleTurn struct {
		Role    string
		Persona string
		Output  string
	}

	// Round 1: Individual Proposals & Framing
	round1Outputs := make([]roleTurn, 0, len(c.members))
	for _, m := range c.members {
		if c.streamHandler != nil {
			c.streamHandler(CouncilStreamEvent{
				Type:        "council_round",
				Timestamp:   time.Now().UTC(),
				SpecID:      specID,
				Round:       1,
				TotalRounds: 3,
				Role:        m.Role,
				PersonaID:   m.Persona.ID,
				Status:      "in_progress",
				Message:     fmt.Sprintf("Round 1: Deliberating with %s (%s)", m.Role, m.Persona.Name),
			})
		}
		sysPrompt := fmt.Sprintf("You are %s (%s). Deliberate on the requested feature. Provide your role's specific requirements, constraints, architectural boundaries, edge cases, and acceptance criteria.", m.Role, m.Persona.Name)
		userPrompt := fmt.Sprintf("FEATURE REQUEST: %s\nStyle Profile: %s\nProvide your analysis.", pCtx.StoryPrompt, pCtx.Style)

		out, err := c.deliberator(ctx, m.Role, m.Persona, sysPrompt, userPrompt)
		if err != nil {
			return nil, fmt.Errorf("council deliberation failed at %s: %w", m.Role, err)
		}
		round1Outputs = append(round1Outputs, roleTurn{Role: m.Role, Persona: m.Persona.Name, Output: out})
	}

	// Round 2: Adversarial Critique & Cross-Role Review
	var round1Summary strings.Builder
	for _, t := range round1Outputs {
		fmt.Fprintf(&round1Summary, "### %s (%s):\n%s\n\n", t.Role, t.Persona, t.Output)
	}

	if c.streamHandler != nil {
		c.streamHandler(CouncilStreamEvent{
			Type:        "council_round",
			Timestamp:   time.Now().UTC(),
			SpecID:      specID,
			Round:       2,
			TotalRounds: 3,
			Role:        "Senior Architect & QA Lead",
			PersonaID:   "senior_software_architect",
			Status:      "in_progress",
			Message:     "Round 2: Reconciling cross-role conflicts and defining Gherkin test boundaries",
		})
	}

	critiqueSysPrompt := "You are the Senior Software Architect and QA Lead reconciling stakeholder positions. Identify conflicts, ensure layer isolation, and establish strict Gherkin criteria."
	critiqueUserPrompt := fmt.Sprintf("Review stakeholder proposals for feature %q:\n\n%s\nReconcile conflicts and eliminate ambiguity.", pCtx.StoryPrompt, round1Summary.String())

	critiqueOutput := ""
	architectPersona := c.getMemberPersona("senior_software_architect")
	crit, err := c.deliberator(ctx, "Council Synthesis", architectPersona, critiqueSysPrompt, critiqueUserPrompt)
	if err != nil {
		critiqueOutput = fmt.Sprintf("CRITIQUE_FAILED: %v", err)
	} else {
		critiqueOutput = crit
	}

	// Round 3: Synthesis into Canonical JSON Draft by neutral synthesizer
	if c.streamHandler != nil {
		c.streamHandler(CouncilStreamEvent{
			Type:        "council_round",
			Timestamp:   time.Now().UTC(),
			SpecID:      specID,
			Round:       3,
			TotalRounds: 3,
			Role:        "Lead Council Synthesizer",
			PersonaID:   "lead_council_synthesizer",
			Status:      "in_progress",
			Message:     "Round 3: Synthesizing canonical Story Spec JSON and ADR records",
		})
	}

	synthSysPrompt := `You are the Lead Council Synthesizer. You must synthesize the final canonical Story Spec based on the council deliberation.
Output MUST be valid JSON conforming to:
{
  "title": "string",
  "userStory": "string",
  "inScope": ["string"],
  "outOfScope": ["string"],
  "acceptanceCriteria": [{"name": "string", "given": "string", "when": "string", "then": "string"}],
  "decisions": [{"id": "ADR-01", "title": "string", "context": "string", "decision": "string", "consequences": ["string"]}],
  "fileManifest": [{"action": "create|modify|delete", "path": "string", "rationale": "string"}],
  "testCommands": ["string"]
}
Reply with ONLY the JSON object.`

	synthUserPrompt := fmt.Sprintf("Synthesize final Story Spec for %q.\nStyle: %s\nStakeholder Proposals:\n%s\nCritique & Reconciliation:\n%s", pCtx.StoryPrompt, pCtx.Style, round1Summary.String(), critiqueOutput)

	synthesizerPersona := c.getSynthesizerPersona()
	finalRaw, err := c.deliberator(ctx, "Council Synthesizer", synthesizerPersona, synthSysPrompt, synthUserPrompt)
	if err != nil {
		return nil, fmt.Errorf("final council synthesis failed: %w", err)
	}

	if c.streamHandler != nil {
		c.streamHandler(CouncilStreamEvent{
			Type:        "council_done",
			Timestamp:   time.Now().UTC(),
			SpecID:      specID,
			TotalRounds: 3,
			Status:      "completed",
			Message:     fmt.Sprintf("Council deliberation completed for %s", specID),
		})
	}

	delibRounds := []DeliberationRound{
		{
			Round:      1,
			Role:       "Stakeholder Council Members",
			Topic:      "Role Proposals & Framing",
			Transcript: round1Summary.String(),
		},
		{
			Round:      2,
			Role:       "Senior Software Architect & QA Lead",
			Topic:      "Adversarial Critique & Conflict Resolution",
			Transcript: critiqueOutput,
		},
		{
			Round:      3,
			Role:       "Lead Council Synthesizer",
			Topic:      "Synthesis into Canonical StorySpec",
			Transcript: finalRaw,
		},
	}

	draft, err := parseDeliberationDraft(finalRaw)
	if err != nil {
		// If JSON parsing fails, construct spec incorporating the text
		spec := &StorySpec{
			ID:        specID,
			Title:     defaultTitle,
			UserStory: fmt.Sprintf("As a developer/user, I want to %s.", pCtx.StoryPrompt),
			Style:     pCtx.Style,
			InScope:   []string{pCtx.StoryPrompt},
			AcceptanceCriteria: []Scenario{
				{
					Name:  "Deliberated Primary Path",
					Given: "the system is in a valid state",
					When:  pCtx.StoryPrompt,
					Then:  "the requirements agreed by the Stakeholder Council are verified",
				},
			},
			DeliberationRounds: delibRounds,
		}
		c.applyRepoTestCommands(spec, pCtx.RepoContext)
		spec.RawMarkdown = FormatToMarkdown(spec)
		return spec, nil
	}

	title := draft.Title
	if title == "" {
		title = defaultTitle
	}

	spec := &StorySpec{
		ID:                 specID,
		Title:              title,
		UserStory:          draft.UserStory,
		InScope:            draft.InScope,
		OutOfScope:         draft.OutOfScope,
		AcceptanceCriteria: draft.AcceptanceCriteria,
		Decisions:          draft.Decisions,
		FileManifest:       draft.FileManifest,
		TestCommands:       draft.TestCommands,
		Style:              pCtx.Style,
		DeliberationRounds: delibRounds,
	}

	if len(spec.TestCommands) == 0 {
		c.applyRepoTestCommands(spec, pCtx.RepoContext)
	}
	if len(spec.InScope) == 0 {
		spec.InScope = append(spec.InScope, pCtx.StoryPrompt)
	}

	spec.RawMarkdown = FormatToMarkdown(spec)
	return spec, nil
}

func (c *Council) applyRepoTestCommands(spec *StorySpec, repoCtx *repo.RepositoryContext) {
	if repoCtx != nil {
		for _, eco := range repoCtx.DetectedEcos {
			switch eco {
			case repo.EcosystemGo:
				spec.TestCommands = append(spec.TestCommands, "go test -v ./...")
			case repo.EcosystemGradleKMP:
				spec.TestCommands = append(spec.TestCommands, "./gradlew testDebugUnitTest", "./gradlew ktlintCheck")
			case repo.EcosystemRust:
				spec.TestCommands = append(spec.TestCommands, "cargo test", "cargo clippy")
			case repo.EcosystemNode:
				spec.TestCommands = append(spec.TestCommands, "npm test")
			case repo.EcosystemPython:
				spec.TestCommands = append(spec.TestCommands, "pytest")
			}
		}
	}
	if len(spec.TestCommands) == 0 {
		spec.TestCommands = append(spec.TestCommands, "go test ./...")
	}
}

func parseDeliberationDraft(raw string) (*DeliberationDraft, error) {
	var out DeliberationDraft
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start || !utf8.ValidString(raw) {
		return nil, fmt.Errorf("no valid JSON object found in council response")
	}

	dec := json.NewDecoder(strings.NewReader(raw[start : end+1]))
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func deriveTitle(prompt string) string {
	words := strings.Fields(prompt)
	if len(words) > 6 {
		return strings.Join(words[:6], " ") + "..."
	}
	return prompt
}

var lastSpecNumber atomic.Int64

// nextSpecNumber returns a millisecond timestamp that is strictly increasing within the
// process, so two specs planned in the same instant never share an ID (and file name).
func nextSpecNumber() int64 {
	for {
		prev := lastSpecNumber.Load()
		n := time.Now().UnixMilli()
		if n <= prev {
			n = prev + 1
		}
		if lastSpecNumber.CompareAndSwap(prev, n) {
			return n
		}
	}
}
