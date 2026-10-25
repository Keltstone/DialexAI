package hunt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Precedence levels
const (
	SourceDefault = "built-in default"
	SourceOrg     = "org default"
	SourceFile    = "project file (.sentinix/hunt.json)"
	SourceEnv     = "env"
	SourceCLI     = "cli flag"
)

var (
	envVarRefRegex = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)\}$`)
	secretKeyRegex = regexp.MustCompile(`(?i)token|key|secret|password|authorization|credential`)
)

// OrgCeilings defines maximum allowable resources and permitted boundaries set by the organization.
type OrgCeilings struct {
	MaxCalls        int      `json:"max_calls,omitempty"`
	MaxCostUSD      float64  `json:"max_cost_usd,omitempty"`
	MaxTokens       int      `json:"max_tokens,omitempty"`
	AllowedPolicies []string `json:"allowed_policies,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
}

// PassConfig defines one audit pass.
type PassConfig struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

// ModelPrice defines per-million token pricing in USD.
type ModelPrice struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
	CachedPerMillion float64 `json:"cached_per_million"`
}

// ToolsConfig defines tool capabilities and execution limits.
type ToolsConfig struct {
	Enabled      bool              `json:"enabled"`
	StepCap      int               `json:"step_cap,omitempty"`
	SizeCapBytes int               `json:"size_cap_bytes,omitempty"`
	Allowlist    []string          `json:"allowlist,omitempty"`
	MCPServers   map[string]string `json:"mcp_servers,omitempty"`
}

// PoCConfig controls dynamic proof-of-concept reproduction in the sandbox.
type PoCConfig struct {
	Enabled      bool     `json:"enabled"`
	SandboxImage string   `json:"sandbox_image,omitempty"`
	Command      []string `json:"command,omitempty"`
	MaxAttempts  int      `json:"max_attempts,omitempty"`
}

// ValidationConfig configures the multi-stage false-positive reduction pipeline.
type ValidationConfig struct {
	MitigationCheck     bool    `json:"mitigation_check"`
	SeverityCalibration bool    `json:"severity_calibration"`
	Dedupe              bool    `json:"dedupe"`
	MinConfidence       float64 `json:"min_confidence,omitempty"`
	MinExploitability   string  `json:"min_exploitability,omitempty"` // unconfirmed, likely, confirmed
}

// CustomEntrypoint allows user-defined regex and glob entry point discovery.
type CustomEntrypoint struct {
	Name     string `json:"name"`
	FileGlob string `json:"file_glob"`
	Regex    string `json:"regex"`
	Kind     string `json:"kind"`
	Platform string `json:"platform,omitempty"`
}

// ClassConfig adds or overrides a vulnerability class.
type ClassConfig struct {
	ID       string   `json:"id"`
	CWE      string   `json:"cwe,omitempty"`
	Hint     string   `json:"hint,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	MaxSev   string   `json:"max_severity,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
}

// CouncilConfig controls multi-model voting.
type CouncilConfig struct {
	Enabled  bool `json:"enabled"`
	MinVotes int  `json:"min_votes,omitempty"`
}

// HuntConfig is the unified configuration schema for Hunt v2.
type HuntConfig struct {
	Version           int                   `json:"version"`
	Taskflow          string                `json:"taskflow,omitempty"`
	Platform          string                `json:"platform,omitempty"`
	Gate              string                `json:"gate,omitempty"`   // "advisory" (default) or "confirmed-only"
	Policy            string                `json:"policy,omitempty"` // "local-only" (default), "cloud-redacted", "cloud"
	ModelsFile        string                `json:"models_file,omitempty"`
	Roles             map[string]string     `json:"roles,omitempty"`
	MaxCalls          int                   `json:"max_calls,omitempty"`
	MaxTokens         int                   `json:"max_tokens,omitempty"`
	MaxCostUSD        float64               `json:"max_cost_usd,omitempty"`
	MaxWallTime       string                `json:"max_wall_time,omitempty"`
	Prices            map[string]ModelPrice `json:"prices,omitempty"`
	Passes            []PassConfig          `json:"passes,omitempty"`
	StopOnHit         bool                  `json:"stop_on_hit"`
	SamplesK          int                   `json:"samples_k,omitempty"`
	Council           CouncilConfig         `json:"council,omitempty"`
	Tools             ToolsConfig           `json:"tools,omitempty"`
	PoC               PoCConfig             `json:"poc,omitempty"`
	Validation        ValidationConfig      `json:"validation,omitempty"`
	CustomEntrypoints []CustomEntrypoint    `json:"custom_entrypoints,omitempty"`
	Classes           []ClassConfig         `json:"classes,omitempty"`
	HuntPacks         []string              `json:"hunt_packs,omitempty"`
	Trust             string                `json:"trust,omitempty"`
	Variant           string                `json:"variant,omitempty"`
	VariantGHSA       string                `json:"variant_ghsa,omitempty"`
	Resume            string                `json:"resume,omitempty"`
	Only              string                `json:"only,omitempty"`
	DiffAware         bool                  `json:"diff_aware,omitempty"`
	BaseRef           string                `json:"base_ref,omitempty"`
	CodeQL            bool                  `json:"codeql,omitempty"`
	DryRun            bool                  `json:"dry_run,omitempty"`

	// Provenance tracks which source established each field.
	Sources map[string]string `json:"-"`
}

// DefaultHuntConfig returns the baseline configuration with safe defaults.
func DefaultHuntConfig() HuntConfig {
	return HuntConfig{
		Version:     1,
		Taskflow:    "android-audit",
		Platform:    "",
		Gate:        "advisory",
		Policy:      "local-only",
		MaxCalls:    200,
		MaxTokens:   500000,
		MaxCostUSD:  5.0,
		MaxWallTime: "30m",
		StopOnHit:   true,
		SamplesK:    1,
		Council: CouncilConfig{
			Enabled:  false,
			MinVotes: 2,
		},
		Passes: []PassConfig{
			{Name: "strict", Prompt: "Report only a bug that is clearly exploitable by an unprivileged attacker in a realistic state."},
			{Name: "broad", Prompt: "Think creatively about unusual attacker-controlled inputs and chained weaknesses."},
		},
		Tools: ToolsConfig{
			Enabled:      false,
			StepCap:      3,
			SizeCapBytes: 6000,
			Allowlist: []string{
				"read_file", "grep", "entrypoints", "list_dir",
				"read_symbol", "find_references", "manifest_query", "callers",
			},
			MCPServers: map[string]string{},
		},
		PoC: PoCConfig{
			Enabled:     false,
			MaxAttempts: 2,
		},
		Validation: ValidationConfig{
			MitigationCheck:     true,
			SeverityCalibration: true,
			Dedupe:              true,
			MinConfidence:       0.6,
			MinExploitability:   "unconfirmed",
		},
		Sources: make(map[string]string),
	}
}

// InitConfigFile writes a formatted default .sentinix/hunt.json file.
func InitConfigFile(path string) error {
	if path == "" {
		path = filepath.Join(".sentinix", "hunt.json")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config file already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c := DefaultHuntConfig()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// LoadConfigFile parses a JSON config file with strict unknown-field checking.
func LoadConfigFile(path string) (HuntConfig, error) {
	var c HuntConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("config error in %s: %w", path, err)
	}
	return c, nil
}

// Validate checks the configuration against semantic rules, secret protection, and optional org ceilings.
func (c *HuntConfig) Validate(ceilings *OrgCeilings) error {
	var errs []string

	// Version check
	if c.Version < 1 {
		errs = append(errs, "version: must be >= 1")
	}

	// Gate check
	if c.Gate != "" && c.Gate != "advisory" && c.Gate != "confirmed-only" {
		errs = append(errs, fmt.Sprintf("gate: invalid value %q (must be advisory or confirmed-only)", c.Gate))
	}

	// Policy check
	if c.Policy != "" && c.Policy != "local-only" && c.Policy != "cloud-redacted" && c.Policy != "cloud" {
		errs = append(errs, fmt.Sprintf("policy: invalid value %q (must be local-only, cloud-redacted, or cloud)", c.Policy))
	}

	// Wall time check
	if c.MaxWallTime != "" {
		if _, err := time.ParseDuration(c.MaxWallTime); err != nil {
			errs = append(errs, fmt.Sprintf("max_wall_time: invalid duration %q: %v", c.MaxWallTime, err))
		}
	}

	// Budgets
	if c.MaxCalls < 0 {
		errs = append(errs, "max_calls: must be >= 0")
	}
	if c.MaxTokens < 0 {
		errs = append(errs, "max_tokens: must be >= 0")
	}
	if c.MaxCostUSD < 0 {
		errs = append(errs, "max_cost_usd: must be >= 0")
	}

	// Secret check: recursively scan all string fields and maps for literal secrets
	if secErrs := c.validateSecrets(); len(secErrs) > 0 {
		errs = append(errs, secErrs...)
	}

	// Org ceilings check
	if ceilings != nil {
		if ceilings.MaxCalls > 0 && c.MaxCalls > ceilings.MaxCalls {
			errs = append(errs, fmt.Sprintf("max_calls: %d exceeds org ceiling of %d", c.MaxCalls, ceilings.MaxCalls))
		}
		if ceilings.MaxCostUSD > 0 && c.MaxCostUSD > ceilings.MaxCostUSD {
			errs = append(errs, fmt.Sprintf("max_cost_usd: %.2f exceeds org ceiling of %.2f", c.MaxCostUSD, ceilings.MaxCostUSD))
		}
		if ceilings.MaxTokens > 0 && c.MaxTokens > ceilings.MaxTokens {
			errs = append(errs, fmt.Sprintf("max_tokens: %d exceeds org ceiling of %d", c.MaxTokens, ceilings.MaxTokens))
		}
		if len(ceilings.AllowedPolicies) > 0 && c.Policy != "" {
			allowed := false
			for _, p := range ceilings.AllowedPolicies {
				if c.Policy == p {
					allowed = true
					break
				}
			}
			if !allowed {
				errs = append(errs, fmt.Sprintf("policy: %q not permitted by org ceiling (allowed: %s)", c.Policy, strings.Join(ceilings.AllowedPolicies, ", ")))
			}
		}
		if len(ceilings.AllowedTools) > 0 && c.Tools.Enabled {
			for _, t := range c.Tools.Allowlist {
				allowed := false
				for _, at := range ceilings.AllowedTools {
					if t == at {
						allowed = true
						break
					}
				}
				if !allowed {
					errs = append(errs, fmt.Sprintf("tools.allowlist: %q not permitted by org ceiling", t))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateSecrets checks for forbidden literal credentials across all nested fields and maps.
func (c *HuntConfig) validateSecrets() []string {
	var errs []string
	var checkValue func(v reflect.Value, path string)
	checkValue = func(v reflect.Value, path string) {
		if !v.IsValid() {
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			t := v.Type()
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				subPath := field.Name
				if path != "" {
					subPath = path + "." + field.Name
				}
				checkValue(v.Field(i), subPath)
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				k := iter.Key().String()
				val := iter.Value()
				subPath := fmt.Sprintf("%s[%q]", path, k)
				if val.Kind() == reflect.String {
					vs := val.String()
					if secretKeyRegex.MatchString(k) && vs != "" && !envVarRefRegex.MatchString(vs) {
						errs = append(errs, fmt.Sprintf("literal secret in %s: use a ${ENV_VAR} reference", subPath))
					}
				} else {
					checkValue(val, subPath)
				}
			}
		case reflect.String:
			s := v.String()
			fieldName := path
			if idx := strings.LastIndex(path, "."); idx >= 0 {
				fieldName = path[idx+1:]
			}
			if secretKeyRegex.MatchString(fieldName) && s != "" && !envVarRefRegex.MatchString(s) {
				errs = append(errs, fmt.Sprintf("literal secret in %s: use a ${ENV_VAR} reference", path))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				checkValue(v.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}

	checkValue(reflect.ValueOf(*c), "")
	return errs
}

// IsLooseningSafeguards reports whether moving from oldCfg to newCfg weakens security guarantees.
// A loosening change requires secondary authorization and audit log recording.
func IsLooseningSafeguards(oldCfg, newCfg HuntConfig, org *OrgCeilings) (bool, string) {
	// 1. Raising gate from advisory to confirmed-only
	if oldCfg.Gate != "confirmed-only" && newCfg.Gate == "confirmed-only" {
		return true, "enabling confirmed-only blocking gate"
	}
	// 2. Widening model policy (local-only -> cloud-redacted or cloud, or cloud-redacted -> cloud)
	policyRank := map[string]int{"local-only": 1, "cloud-redacted": 2, "cloud": 3}
	if policyRank[newCfg.Policy] > policyRank[oldCfg.Policy] {
		return true, fmt.Sprintf("widening model policy from %s to %s", oldCfg.Policy, newCfg.Policy)
	}
	// 3. Enabling tools when previously disabled
	if !oldCfg.Tools.Enabled && newCfg.Tools.Enabled {
		return true, "enabling tool execution"
	}
	// 4. Enabling PoC runner when previously disabled
	if !oldCfg.PoC.Enabled && newCfg.PoC.Enabled {
		return true, "enabling PoC sandbox test execution"
	}
	// 5. Exceeding org limits
	if org != nil {
		if org.MaxCalls > 0 && newCfg.MaxCalls > org.MaxCalls {
			return true, fmt.Sprintf("raising max_calls (%d) above org ceiling (%d)", newCfg.MaxCalls, org.MaxCalls)
		}
		if org.MaxCostUSD > 0 && newCfg.MaxCostUSD > org.MaxCostUSD {
			return true, fmt.Sprintf("raising max_cost_usd (%.2f) above org ceiling (%.2f)", newCfg.MaxCostUSD, org.MaxCostUSD)
		}
	}
	return false, ""
}

// CLIFlags holds parsed command-line flags for overriding HuntConfig.
type CLIFlags struct {
	AI           string
	Policy       string
	MaxCalls     int
	SandboxImage string
	PoCCmd       string
	HuntPacks    string
	Trust        string
	Variant      string
	VariantGHSA  string
	Tools        *bool
	ToolSteps    int
	Platform     string
	Resume       string
	Only         string
	Gate         string
	Taskflow     string
	DryRun       bool
	CodeQL       *bool
	Diff         bool
}

// ResolveConfig computes the effective HuntConfig by layering all sources according to strict precedence:
// built-in defaults < org default < project file (.sentinix/hunt.json) < env vars (SENTINIX_HUNT_*) < CLI flags.
func ResolveConfig(root string, orgCfg *HuntConfig, flags CLIFlags) (HuntConfig, error) {
	effective := DefaultHuntConfig()
	sources := make(map[string]string)

	// Record default sources
	recordSource := func(field, src string) {
		sources[field] = src
	}
	for _, f := range []string{
		"version", "gate", "policy", "max_calls", "max_tokens", "max_cost_usd",
		"max_wall_time", "taskflow", "tools", "poc", "validation", "passes",
	} {
		recordSource(f, SourceDefault)
	}

	// 2. Layer Org defaults if provided
	if orgCfg != nil {
		if orgCfg.Gate != "" {
			effective.Gate = orgCfg.Gate
			recordSource("gate", SourceOrg)
		}
		if orgCfg.Policy != "" {
			effective.Policy = orgCfg.Policy
			recordSource("policy", SourceOrg)
		}
		if orgCfg.MaxCalls > 0 {
			effective.MaxCalls = orgCfg.MaxCalls
			recordSource("max_calls", SourceOrg)
		}
		if orgCfg.MaxCostUSD > 0 {
			effective.MaxCostUSD = orgCfg.MaxCostUSD
			recordSource("max_cost_usd", SourceOrg)
		}
	}

	// 3. Layer Project file (.sentinix/hunt.json) if present
	configFile := filepath.Join(root, ".sentinix", "hunt.json")
	if _, err := os.Stat(configFile); err == nil {
		fileCfg, err := LoadConfigFile(configFile)
		if err != nil {
			return effective, err
		}
		if fileCfg.Version > 0 {
			effective.Version = fileCfg.Version
			recordSource("version", SourceFile)
		}
		if fileCfg.Taskflow != "" {
			effective.Taskflow = fileCfg.Taskflow
			recordSource("taskflow", SourceFile)
		}
		if fileCfg.Platform != "" {
			effective.Platform = fileCfg.Platform
			recordSource("platform", SourceFile)
		}
		if fileCfg.Gate != "" {
			effective.Gate = fileCfg.Gate
			recordSource("gate", SourceFile)
		}
		if fileCfg.Policy != "" {
			effective.Policy = fileCfg.Policy
			recordSource("policy", SourceFile)
		}
		if fileCfg.ModelsFile != "" {
			effective.ModelsFile = fileCfg.ModelsFile
			recordSource("models_file", SourceFile)
		}
		if fileCfg.MaxCalls > 0 {
			effective.MaxCalls = fileCfg.MaxCalls
			recordSource("max_calls", SourceFile)
		}
		if fileCfg.MaxTokens > 0 {
			effective.MaxTokens = fileCfg.MaxTokens
			recordSource("max_tokens", SourceFile)
		}
		if fileCfg.MaxCostUSD > 0 {
			effective.MaxCostUSD = fileCfg.MaxCostUSD
			recordSource("max_cost_usd", SourceFile)
		}
		if fileCfg.MaxWallTime != "" {
			effective.MaxWallTime = fileCfg.MaxWallTime
			recordSource("max_wall_time", SourceFile)
		}
		if len(fileCfg.Passes) > 0 {
			effective.Passes = fileCfg.Passes
			recordSource("passes", SourceFile)
		}
		if fileCfg.Tools.Enabled {
			effective.Tools.Enabled = true
			if fileCfg.Tools.StepCap > 0 {
				effective.Tools.StepCap = fileCfg.Tools.StepCap
			}
			if len(fileCfg.Tools.Allowlist) > 0 {
				effective.Tools.Allowlist = fileCfg.Tools.Allowlist
			}
			recordSource("tools", SourceFile)
		}
		if fileCfg.PoC.Enabled {
			effective.PoC = fileCfg.PoC
			recordSource("poc", SourceFile)
		}
		if len(fileCfg.CustomEntrypoints) > 0 {
			effective.CustomEntrypoints = fileCfg.CustomEntrypoints
			recordSource("custom_entrypoints", SourceFile)
		}
		if len(fileCfg.Classes) > 0 {
			effective.Classes = fileCfg.Classes
			recordSource("classes", SourceFile)
		}
		if fileCfg.CodeQL {
			effective.CodeQL = true
			recordSource("codeql", SourceFile)
		}
	}

	// 4. Layer Environment variables (SENTINIX_HUNT_*)
	if v := os.Getenv("SENTINIX_HUNT_POLICY"); v != "" {
		effective.Policy = v
		recordSource("policy", SourceEnv+" (SENTINIX_HUNT_POLICY)")
	}
	if v := os.Getenv("SENTINIX_HUNT_GATE"); v != "" {
		effective.Gate = v
		recordSource("gate", SourceEnv+" (SENTINIX_HUNT_GATE)")
	}
	if v := os.Getenv("SENTINIX_HUNT_MAX_CALLS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			effective.MaxCalls = n
			recordSource("max_calls", SourceEnv+" (SENTINIX_HUNT_MAX_CALLS)")
		}
	}
	if v := os.Getenv("SENTINIX_HUNT_MAX_COST"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			effective.MaxCostUSD = f
			recordSource("max_cost_usd", SourceEnv+" (SENTINIX_HUNT_MAX_COST)")
		}
	}
	if v := os.Getenv("SENTINIX_HUNT_TASKFLOW"); v != "" {
		effective.Taskflow = v
		recordSource("taskflow", SourceEnv+" (SENTINIX_HUNT_TASKFLOW)")
	}
	if v := os.Getenv("SENTINIX_CODEQL"); v == "1" || v == "true" {
		effective.CodeQL = true
		recordSource("codeql", SourceEnv+" (SENTINIX_CODEQL)")
	}

	// 5. Layer CLI flags (highest priority)
	if flags.AI != "" {
		effective.ModelsFile = flags.AI
		recordSource("models_file", SourceCLI+" (--ai)")
	}
	if flags.Policy != "" {
		effective.Policy = flags.Policy
		recordSource("policy", SourceCLI+" (--policy)")
	}
	if flags.MaxCalls > 0 {
		effective.MaxCalls = flags.MaxCalls
		recordSource("max_calls", SourceCLI+" (--max-calls)")
	}
	if flags.Gate != "" {
		effective.Gate = flags.Gate
		recordSource("gate", SourceCLI+" (--gate)")
	}
	if flags.Taskflow != "" {
		effective.Taskflow = flags.Taskflow
		recordSource("taskflow", SourceCLI+" (--taskflow)")
	}
	if flags.Platform != "" {
		effective.Platform = flags.Platform
		recordSource("platform", SourceCLI+" (--platform)")
	}
	if flags.Tools != nil {
		effective.Tools.Enabled = *flags.Tools
		recordSource("tools", SourceCLI+" (--tools)")
	}
	if flags.ToolSteps > 0 {
		effective.Tools.StepCap = flags.ToolSteps
		recordSource("tools.step_cap", SourceCLI+" (--tool-steps)")
	}
	if flags.SandboxImage != "" || flags.PoCCmd != "" {
		effective.PoC.Enabled = true
		if flags.SandboxImage != "" {
			effective.PoC.SandboxImage = flags.SandboxImage
		}
		if flags.PoCCmd != "" {
			effective.PoC.Command = strings.Fields(flags.PoCCmd)
		}
		recordSource("poc", SourceCLI+" (--sandbox-image/--poc-cmd)")
	}
	if flags.HuntPacks != "" {
		effective.HuntPacks = []string{flags.HuntPacks}
		recordSource("hunt_packs", SourceCLI+" (--hunt-packs)")
	}
	if flags.Trust != "" {
		effective.Trust = flags.Trust
		recordSource("trust", SourceCLI+" (--trust)")
	}
	if flags.Variant != "" {
		effective.Variant = flags.Variant
		recordSource("variant", SourceCLI+" (--variant)")
	}
	if flags.VariantGHSA != "" {
		effective.VariantGHSA = flags.VariantGHSA
		recordSource("variant_ghsa", SourceCLI+" (--variant-ghsa)")
	}
	if flags.Resume != "" {
		effective.Resume = flags.Resume
		recordSource("resume", SourceCLI+" (--resume)")
	}
	if flags.Only != "" {
		effective.Only = flags.Only
		recordSource("only", SourceCLI+" (--only)")
	}
	if flags.CodeQL != nil {
		effective.CodeQL = *flags.CodeQL
		recordSource("codeql", SourceCLI+" (--codeql)")
	}
	if flags.DryRun {
		effective.DryRun = true
		recordSource("dry_run", SourceCLI+" (--dry-run)")
	}

	effective.Sources = sources
	return effective, nil
}

// Explain prints each effective configuration field alongside the source that set it.
func (c *HuntConfig) Explain() string {
	var keys []string
	for k := range c.Sources {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("EFFECTIVE HUNT CONFIGURATION:\n")
	for _, k := range keys {
		src := c.Sources[k]
		val := c.fieldValueString(k)
		sb.WriteString(fmt.Sprintf("  %-18s = %-25s [%s]\n", k, val, src))
	}
	return sb.String()
}

func (c *HuntConfig) fieldValueString(field string) string {
	switch field {
	case "version":
		return strconv.Itoa(c.Version)
	case "gate":
		return c.Gate
	case "policy":
		return c.Policy
	case "taskflow":
		return c.Taskflow
	case "platform":
		if c.Platform == "" {
			return "(all)"
		}
		return c.Platform
	case "max_calls":
		return strconv.Itoa(c.MaxCalls)
	case "max_tokens":
		return strconv.Itoa(c.MaxTokens)
	case "max_cost_usd":
		return fmt.Sprintf("$%.2f", c.MaxCostUSD)
	case "max_wall_time":
		return c.MaxWallTime
	case "models_file":
		return c.ModelsFile
	case "tools":
		return fmt.Sprintf("enabled=%v", c.Tools.Enabled)
	case "tools.step_cap":
		return strconv.Itoa(c.Tools.StepCap)
	case "poc":
		return fmt.Sprintf("enabled=%v", c.PoC.Enabled)
	case "codeql":
		return strconv.FormatBool(c.CodeQL)
	case "dry_run":
		return strconv.FormatBool(c.DryRun)
	default:
		return "..."
	}
}
