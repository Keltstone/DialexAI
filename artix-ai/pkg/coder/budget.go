package coder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"artix/pkg/policy"
	"socratix/pkg/model"
)

// TaskType defines the operational context for compiling Persona DNA layers.
type TaskType string

const (
	TaskCodeGeneration   TaskType = "code_generation"
	TaskPatchReview      TaskType = "patch_review"
	TaskSpecDeliberation TaskType = "spec_deliberation"
	TaskSecurityAudit    TaskType = "security_audit"
)

// TokenBudget defines token and USD financial limits at story, team, and daily levels.
type TokenBudget struct {
	MaxStoryTokens  int                `json:"maxStoryTokens"`
	MaxTeamTokens   int                `json:"maxTeamTokens"`
	MaxDayTokens    int                `json:"maxDayTokens"`
	MaxStoryCost    float64            `json:"maxStoryCost,omitempty"` // USD
	MaxTeamCost     float64            `json:"maxTeamCost,omitempty"`  // USD
	MaxDayCost      float64            `json:"maxDayCost,omitempty"`   // USD
	CostPer1kTokens float64            `json:"costPer1kTokens,omitempty"`
	PriceTable      map[string]float64 `json:"priceTable,omitempty"` // model -> cost per 1k tokens
	UsedStoryTokens int                `json:"usedStoryTokens"`
	UsedTeamTokens  int                `json:"usedTeamTokens"`
	UsedDayTokens   int                `json:"usedDayTokens"`
	UsedStoryCost   float64            `json:"usedStoryCost"`
	UsedTeamCost    float64            `json:"usedTeamCost"`
	UsedDayCost     float64            `json:"usedDayCost"`
	TeamID          string             `json:"teamId,omitempty"`
	TaskID          string             `json:"taskId,omitempty"`
	LedgerPath      string             `json:"ledgerPath,omitempty"`
	LedgerError     error              `json:"-"`
}

// LoadBudgetFromEnv loads budget limits from policy and ARTIX_BUDGET_* environment variables.
func LoadBudgetFromEnv() *TokenBudget {
	b := &TokenBudget{}
	hasConfig := false

	// Load from verified policy first
	pol := policy.Active()
	if pol != nil {
		if pol.Budget.MaxStoryTokens > 0 {
			b.MaxStoryTokens = pol.Budget.MaxStoryTokens
			hasConfig = true
		}
		if pol.Budget.MaxTeamTokens > 0 {
			b.MaxTeamTokens = pol.Budget.MaxTeamTokens
			hasConfig = true
		}
		if pol.Budget.MaxDayTokens > 0 {
			b.MaxDayTokens = pol.Budget.MaxDayTokens
			hasConfig = true
		}
		if pol.Budget.MaxStoryCost > 0 {
			b.MaxStoryCost = pol.Budget.MaxStoryCost
			hasConfig = true
		}
		if pol.Budget.MaxTeamCost > 0 {
			b.MaxTeamCost = pol.Budget.MaxTeamCost
			hasConfig = true
		}
		if pol.Budget.MaxDayCost > 0 {
			b.MaxDayCost = pol.Budget.MaxDayCost
			hasConfig = true
		}
		if len(pol.Budget.PriceTable) > 0 {
			b.PriceTable = make(map[string]float64)
			for k, v := range pol.Budget.PriceTable {
				b.PriceTable[k] = v
			}
			hasConfig = true
		}
		if pol.Budget.CostPer1kTokens > 0 {
			b.CostPer1kTokens = pol.Budget.CostPer1kTokens
			hasConfig = true
		}
	}

	// Environment variable overrides
	if v := getEnv("ARTIX_BUDGET_PER_STORY", "KRITIX_BUDGET_PER_STORY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.MaxStoryTokens = n
			hasConfig = true
		}
	}
	if v := getEnv("ARTIX_BUDGET_PER_TEAM", "KRITIX_BUDGET_PER_TEAM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.MaxTeamTokens = n
			hasConfig = true
		}
	}
	if v := getEnv("ARTIX_BUDGET_PER_DAY", "KRITIX_BUDGET_PER_DAY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.MaxDayTokens = n
			hasConfig = true
		}
	}
	if v := getEnv("ARTIX_BUDGET_STORY_COST", "KRITIX_BUDGET_STORY_COST"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			b.MaxStoryCost = f
			hasConfig = true
		}
	}
	if v := getEnv("ARTIX_BUDGET_TEAM_COST", "KRITIX_BUDGET_TEAM_COST"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			b.MaxTeamCost = f
			hasConfig = true
		}
	}
	if v := getEnv("ARTIX_BUDGET_DAY_COST", "KRITIX_BUDGET_DAY_COST"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			b.MaxDayCost = f
			hasConfig = true
		}
	}
	if v := getEnv("ARTIX_TEAM_ID", "KRITIX_TEAM_ID"); v != "" {
		b.TeamID = v
	}
	if v := getEnv("ARTIX_BUDGET_LEDGER", "KRITIX_BUDGET_LEDGER"); v != "" {
		b.LedgerPath = v
	}

	if !hasConfig {
		return nil
	}
	b.SyncWithSharedLedger()
	return b
}

var ledgerMu sync.Mutex

func withFileLock(path string, fn func() error) error {
	ledgerMu.Lock()
	defer ledgerMu.Unlock()

	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create ledger directory %s: %w", dir, err)
		}
	}

	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire flock on %s: %w", lockPath, err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}

// HasFinancialCap returns true if any USD budget cap is configured.
func (b *TokenBudget) HasFinancialCap() bool {
	return b.MaxStoryCost > 0 || b.MaxTeamCost > 0 || b.MaxDayCost > 0
}

// ValidatePricing verifies that pricing is configured when a financial cap is active.
func (b *TokenBudget) ValidatePricing() error {
	if !b.HasFinancialCap() {
		return nil
	}
	if b.CostPer1kTokens <= 0 && len(b.PriceTable) == 0 {
		return fmt.Errorf("budget configuration error: USD financial cap is configured, but no model pricing or price table is provided (refusing operation to prevent silent under-reporting)")
	}
	return nil
}

// GetModelPrice returns the cost per 1k tokens for the given model key deterministically.
// If a financial cap is active and the model key is not configured, it returns an error refusing operation.
func (b *TokenBudget) GetModelPrice(modelKey string) (float64, error) {
	if b == nil {
		return 0, nil
	}
	trimmedKey := strings.TrimSpace(modelKey)
	if len(b.PriceTable) > 0 {
		if p, ok := b.PriceTable[trimmedKey]; ok && p > 0 {
			return p, nil
		}
		for k, p := range b.PriceTable {
			if strings.EqualFold(k, trimmedKey) && p > 0 {
				return p, nil
			}
		}
		if b.HasFinancialCap() {
			return 0, fmt.Errorf("budget error: model %q missing from price table (refusing operation to prevent silent under-reporting under USD financial cap)", modelKey)
		}
	}
	if b.CostPer1kTokens > 0 {
		return b.CostPer1kTokens, nil
	}
	if b.HasFinancialCap() {
		return 0, fmt.Errorf("budget error: no price table or pricing configured for model %q under active USD financial cap (refusing operation)", modelKey)
	}
	return 0, nil
}

// CheckExhaustion checks if any token or USD cap is exceeded or if ledger operations failed with a cap active.
func (b *TokenBudget) CheckExhaustion() (bool, string) {
	if b.LedgerError != nil && (b.HasFinancialCap() || b.MaxStoryTokens > 0 || b.MaxTeamTokens > 0 || b.MaxDayTokens > 0) {
		return true, fmt.Sprintf("budget aborted due to ledger failure: %v", b.LedgerError)
	}
	if b.MaxStoryTokens > 0 && b.UsedStoryTokens > b.MaxStoryTokens {
		return true, fmt.Sprintf("story token budget exceeded: %d used > %d max cap", b.UsedStoryTokens, b.MaxStoryTokens)
	}
	if b.MaxTeamTokens > 0 && b.UsedTeamTokens > b.MaxTeamTokens {
		return true, fmt.Sprintf("team token budget exceeded: %d used > %d max cap", b.UsedTeamTokens, b.MaxTeamTokens)
	}
	if b.MaxDayTokens > 0 && b.UsedDayTokens > b.MaxDayTokens {
		return true, fmt.Sprintf("daily token budget exceeded: %d used > %d max cap", b.UsedDayTokens, b.MaxDayTokens)
	}
	if b.MaxStoryCost > 0 && b.UsedStoryCost > b.MaxStoryCost {
		return true, fmt.Sprintf("story cost budget exceeded: $%.4f used > $%.4f max cap", b.UsedStoryCost, b.MaxStoryCost)
	}
	if b.MaxTeamCost > 0 && b.UsedTeamCost > b.MaxTeamCost {
		return true, fmt.Sprintf("team cost budget exceeded: $%.4f used > $%.4f max cap", b.UsedTeamCost, b.MaxTeamCost)
	}
	if b.MaxDayCost > 0 && b.UsedDayCost > b.MaxDayCost {
		return true, fmt.Sprintf("daily cost budget exceeded: $%.4f used > $%.4f max cap", b.UsedDayCost, b.MaxDayCost)
	}
	return false, ""
}

// TaskLedgerEntry records total token consumption and USD cost for a specific task.
type TaskLedgerEntry struct {
	Tokens int64   `json:"tokens"`
	USD    float64 `json:"usd"`
}

type sharedLedgerData struct {
	TeamTokens map[string]int              `json:"teamTokens"`
	TeamCost   map[string]float64          `json:"teamCost"`
	DayTokens  map[string]int              `json:"dayTokens"`
	DayCost    map[string]float64          `json:"dayCost"`
	Tasks      map[string]TaskLedgerEntry  `json:"tasks"`
}

func (b *TokenBudget) getEffectiveLedgerPath() string {
	if b.LedgerPath != "" {
		return b.LedgerPath
	}
	if p := os.Getenv("ARTIX_BUDGET_LEDGER"); p != "" {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dir := filepath.Join(home, ".artix")
		_ = os.MkdirAll(dir, 0700)
		return filepath.Join(dir, "budget-ledger.json")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("artix-budget-ledger-%d.json", os.Getuid()))
}

// ValidateLedgerSecurity verifies that the ledger file has secure permissions (mode 0600) and is user-owned.
func (b *TokenBudget) ValidateLedgerSecurity() error {
	path := b.getEffectiveLedgerPath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	mode := info.Mode()
	if mode.Perm()&0077 != 0 {
		return fmt.Errorf("insecure ledger file permissions %04o on %s: group/other access forbidden; must be mode 0600 and user-owned", mode.Perm(), path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != os.Getuid() {
			return fmt.Errorf("ledger file %s owned by uid %d, expected current user uid %d (refusing untrusted ledger)", path, stat.Uid, os.Getuid())
		}
	}
	return nil
}

// SyncWithSharedLedger refreshes UsedTeamTokens, UsedDayTokens, UsedTeamCost, UsedDayCost
// from the atomic shared cross-process ledger file using kernel file locking.
func (b *TokenBudget) SyncWithSharedLedger() {
	if err := b.ValidateLedgerSecurity(); err != nil {
		b.LedgerError = err
		return
	}
	path := b.getEffectiveLedgerPath()
	errLock := withFileLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			return nil
		}

		var state sharedLedgerData
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}

		today := time.Now().Format("2006-01-02")
		teamID := b.TeamID
		if teamID == "" {
			teamID = "default"
		}

		if state.TeamTokens != nil {
			b.UsedTeamTokens = state.TeamTokens[teamID]
		}
		if state.TeamCost != nil {
			b.UsedTeamCost = state.TeamCost[teamID]
		}
		if state.DayTokens != nil {
			b.UsedDayTokens = state.DayTokens[today]
		}
		if state.DayCost != nil {
			b.UsedDayCost = state.DayCost[today]
		}
		return nil
	})
	if errLock != nil {
		b.LedgerError = errLock
	}
}

// ProviderUsage contains actual metered token consumption reported by the model provider API.
type ProviderUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// RecordRoundUsage atomically records token usage into story, team, and day counters,
// reconciling with provider-reported usage when available, and atomically updates the shared ledger
// under an OS kernel file lock.
func (b *TokenBudget) RecordRoundUsage(roundTokens int, roundCostUSD float64, providerUsage ...*ProviderUsage) {
	if err := b.ValidateLedgerSecurity(); err != nil {
		b.LedgerError = err
		return
	}

	// Reconcile against provider-reported usage if present
	if len(providerUsage) > 0 && providerUsage[0] != nil {
		pu := providerUsage[0]
		if pu.TotalTokens > 0 {
			effectiveUnitCost := 0.0
			if roundTokens > 0 {
				effectiveUnitCost = roundCostUSD / float64(roundTokens)
			}
			roundTokens = pu.TotalTokens
			if effectiveUnitCost > 0 {
				roundCostUSD = float64(roundTokens) * effectiveUnitCost
			} else if b.CostPer1kTokens > 0 {
				roundCostUSD = (float64(roundTokens) / 1000.0) * b.CostPer1kTokens
			}
		}
	}

	b.UsedStoryTokens += roundTokens
	b.UsedStoryCost += roundCostUSD

	path := b.getEffectiveLedgerPath()
	errLock := withFileLock(path, func() error {
		var state sharedLedgerData
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &state)
		}

		if state.TeamTokens == nil {
			state.TeamTokens = make(map[string]int)
		}
		if state.TeamCost == nil {
			state.TeamCost = make(map[string]float64)
		}
		if state.DayTokens == nil {
			state.DayTokens = make(map[string]int)
		}
		if state.DayCost == nil {
			state.DayCost = make(map[string]float64)
		}
		if state.Tasks == nil {
			state.Tasks = make(map[string]TaskLedgerEntry)
		}

		today := time.Now().Format("2006-01-02")
		teamID := b.TeamID
		if teamID == "" {
			teamID = "default"
		}

		state.TeamTokens[teamID] += roundTokens
		state.TeamCost[teamID] += roundCostUSD
		state.DayTokens[today] += roundTokens
		state.DayCost[today] += roundCostUSD

		if b.TaskID != "" {
			curTask := state.Tasks[b.TaskID]
			curTask.Tokens += int64(roundTokens)
			curTask.USD += roundCostUSD
			state.Tasks[b.TaskID] = curTask
		}

		b.UsedTeamTokens = state.TeamTokens[teamID]
		b.UsedTeamCost = state.TeamCost[teamID]
		b.UsedDayTokens = state.DayTokens[today]
		b.UsedDayCost = state.DayCost[today]

		marshaled, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		if dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return err
			}
		}
		if err := os.WriteFile(path, marshaled, 0600); err != nil {
			return err
		}
		return os.Chmod(path, 0600)
	})
	if errLock != nil {
		b.LedgerError = errLock
	}
}

func getEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// EstimateTokens calculates an approximate token count based on string length and content.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	// A standard heuristic: ~4 characters per token
	count := (utf8.RuneCountInString(s) + 3) / 4
	if count < 1 {
		return 1
	}
	return count
}

// RoundCost tracks token metrics for a single round of the convergence loop.
type RoundCost struct {
	Round            int            `json:"round"`
	RoleTokens       map[string]int `json:"roleTokens"`
	DNAOverhead      map[string]int `json:"dnaOverhead"`
	TotalRoundTokens int            `json:"totalRoundTokens"`
}

// CostReport is the structured summary of token consumption across the convergence loop.
type CostReport struct {
	StoryID            string      `json:"storyId"`
	TeamID             string      `json:"teamId,omitempty"`
	TotalTokens        int         `json:"totalTokens"`
	TotalCost          float64     `json:"totalCost,omitempty"`
	BudgetStoryCap     int         `json:"budgetStoryCap,omitempty"`
	BudgetTeamCap      int         `json:"budgetTeamCap,omitempty"`
	BudgetDayCap       int         `json:"budgetDayCap,omitempty"`
	BudgetStoryCostCap float64     `json:"budgetStoryCostCap,omitempty"`
	BudgetTeamCostCap  float64     `json:"budgetTeamCostCap,omitempty"`
	BudgetDayCostCap   float64     `json:"budgetDayCostCap,omitempty"`
	Exhausted          bool        `json:"exhausted"`
	ExhaustionReason   string      `json:"exhaustionReason,omitempty"`
	Rounds             []RoundCost `json:"rounds"`
	DNAOverheadTokens  int         `json:"dnaOverheadTokens"`
}

// CompileDNALayers renders only the DNA layers relevant to the given task type.
// For example, patch review omits Communication Style, Temporal Mode, etc.
func CompileDNALayers(dna *model.PersonaDNA, task TaskType) string {
	if dna == nil {
		return ""
	}
	var sb strings.Builder

	// Core Identity: Domain Authority is essential across all tasks
	if dna.CoreIdentity.DomainAuthority != "" {
		fmt.Fprintf(&sb, "Domain Authority: %s\n", dna.CoreIdentity.DomainAuthority)
	}

	// Taboo Space: Forbidden arguments apply to all tasks
	if len(dna.TabooSpace.ForbiddenArguments) > 0 {
		sb.WriteString("\nSTRICT ARCHITECTURAL TABOOS (DO NOT VIOLATE):\n")
		for _, taboo := range dna.TabooSpace.ForbiddenArguments {
			fmt.Fprintf(&sb, "- %s\n", taboo)
		}
	}

	switch task {
	case TaskCodeGeneration:
		// Code generation uses heuristic rules, domain ontology standards, and synthesis preference
		if len(dna.HeuristicLibrary) > 0 {
			sb.WriteString("\nDOMAIN HEURISTICS & RULES:\n")
			for _, h := range dna.HeuristicLibrary {
				fmt.Fprintf(&sb, "- [%s] %s\n", h.Name, h.FormulaOrMaxime)
			}
		}
		if len(dna.DomainOntology.MandatoryStandards) > 0 {
			sb.WriteString("\nMANDATORY STANDARDS:\n")
			for _, s := range dna.DomainOntology.MandatoryStandards {
				fmt.Fprintf(&sb, "- %s\n", s)
			}
		}
		if dna.SynthesisPreference.Style != "" {
			fmt.Fprintf(&sb, "Synthesis Style: %s\n", dna.SynthesisPreference.Style)
		}
		// Explicitly omits: CommunicationVector (tone, sentence ceiling, rhetorical devices)

	case TaskPatchReview:
		// Reviewer requires adversarial posture, mandatory standards, and taboo space, but omits communication style
		if dna.AdversarialPosture.Stance != "" {
			fmt.Fprintf(&sb, "Reviewer Posture: %s (Tenacity: %.2f)\n", dna.AdversarialPosture.Stance, dna.AdversarialPosture.TenacityScore)
		}
		if len(dna.DomainOntology.MandatoryStandards) > 0 {
			sb.WriteString("\nREVIEW COMPLIANCE STANDARDS:\n")
			for _, s := range dna.DomainOntology.MandatoryStandards {
				fmt.Fprintf(&sb, "- %s\n", s)
			}
		}
		// Explicitly omits: CommunicationVector, EpistemicBias theory ratio

	case TaskSpecDeliberation:
		// Council deliberation uses epistemic bias, communication vector, and synthesis preference
		if dna.EpistemicBias.PrimaryMode != "" {
			fmt.Fprintf(&sb, "Reasoning Mode: %s\n", dna.EpistemicBias.PrimaryMode)
		}
		if dna.CommunicationVector.Tone != "" {
			fmt.Fprintf(&sb, "Communication Tone: %s\n", dna.CommunicationVector.Tone)
		}
		if dna.SynthesisPreference.Style != "" {
			fmt.Fprintf(&sb, "Synthesis Style: %s\n", dna.SynthesisPreference.Style)
		}

	case TaskSecurityAudit:
		if dna.AdversarialPosture.Stance != "" {
			fmt.Fprintf(&sb, "Audit Posture: %s\n", dna.AdversarialPosture.Stance)
		}
		if len(dna.DomainOntology.MandatoryStandards) > 0 {
			sb.WriteString("\nAUDIT MANDATORY STANDARDS:\n")
			for _, ms := range dna.DomainOntology.MandatoryStandards {
				fmt.Fprintf(&sb, "- %s\n", ms)
			}
		}
	}

	return sb.String()
}
