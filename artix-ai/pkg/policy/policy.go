package policy

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"artix/internal/forgesec"
)

// DefaultPolicyPath is the build-time default location for enterprise policy.
var DefaultPolicyPath = "/etc/artix/policy.json"

// CompiledTrustedSigningKey can be set at compile time via -ldflags "-X artix/pkg/policy.CompiledTrustedSigningKey=..."
var CompiledTrustedSigningKey = ""

// CompiledTrustedPublicKeyHex can be set at compile time via -ldflags "-X artix/pkg/policy.CompiledTrustedPublicKeyHex=..."
var CompiledTrustedPublicKeyHex = ""

func init() {
	if CompiledTrustedSigningKey != "" {
		SetTrustedKey("compiled-root", CompiledTrustedSigningKey)
	}
	if CompiledTrustedPublicKeyHex != "" {
		_ = SetTrustedPublicKeyHex("compiled-root-pub", CompiledTrustedPublicKeyHex)
	}
}

// TrustedSigningKeys holds symmetric and asymmetric keys trusted for policy signature verification.
var (
	trustedKeysMu     sync.RWMutex
	trustedKeys       = make(map[string]string)            // keyID -> symmetric secret
	trustedPublicKeys = make(map[string]ed25519.PublicKey) // keyID -> ed25519 public key
)

// SetTrustedKey registers a trusted symmetric key for verifying signed policy files.
func SetTrustedKey(keyID, secret string) {
	trustedKeysMu.Lock()
	defer trustedKeysMu.Unlock()
	trustedKeys[keyID] = secret
}

// SetTrustedPublicKey registers an Ed25519 public key trusted for verifying signed policies.
func SetTrustedPublicKey(keyID string, pubKey ed25519.PublicKey) {
	trustedKeysMu.Lock()
	defer trustedKeysMu.Unlock()
	trustedPublicKeys[keyID] = pubKey
}

// SetTrustedPublicKeyHex registers an Ed25519 public key from a hex-encoded string.
func SetTrustedPublicKeyHex(keyID, pubKeyHex string) error {
	raw, err := hex.DecodeString(strings.TrimSpace(pubKeyHex))
	if err != nil {
		return fmt.Errorf("invalid ed25519 public key hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key length %d (expected %d)", len(raw), ed25519.PublicKeySize)
	}
	SetTrustedPublicKey(keyID, ed25519.PublicKey(raw))
	return nil
}

// RemoteSinkConfig defines configuration for an external audit sink (HTTP or syslog).
type RemoteSinkConfig struct {
	Type     string            `json:"type"` // "http" or "syslog"
	Endpoint string            `json:"endpoint"`
	AuthKey  string            `json:"authKey,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// BudgetConfig defines financial and token caps.
type BudgetConfig struct {
	MaxStoryTokens  int                `json:"maxStoryTokens,omitempty"`
	MaxTeamTokens   int                `json:"maxTeamTokens,omitempty"`
	MaxDayTokens    int                `json:"maxDayTokens,omitempty"`
	MaxStoryCost    float64            `json:"maxStoryCost,omitempty"` // USD
	MaxTeamCost     float64            `json:"maxTeamCost,omitempty"`  // USD
	MaxDayCost      float64            `json:"maxDayCost,omitempty"`   // USD
	PriceTable      map[string]float64 `json:"priceTable,omitempty"`   // model -> cost per 1k tokens
	CostPer1kTokens float64            `json:"costPer1kTokens,omitempty"`
}

// ReviewerPolicyConfig defines constraints for adversarial reviewer models.
type ReviewerPolicyConfig struct {
	EnforceDisjointModelFamilies bool              `json:"enforceDisjointModelFamilies"`
	AllowedCriticFamilies        []string          `json:"allowedCriticFamilies,omitempty"`
	AllowedSemanticRunners       []string          `json:"allowedSemanticRunners,omitempty"`
	RestrictedPaths              []string          `json:"restrictedPaths,omitempty"`
	ModelFamilies                map[string]string `json:"modelFamilies,omitempty"`
	ModelDerivations             map[string]string `json:"modelDerivations,omitempty"`
}

var defaultAllowedSemanticRunners = []string{
	"detekt", "konsist", "semgrep", "swiftlint", "eslint", "mypy",
	"ruff", "flake8", "golangci-lint", "govet", "go vet", "checkstyle",
	"ktlint", "bandit", "rubocop", "shellcheck",
}

// IsAllowedSemanticRunner checks whether a command uses an allowlisted static analysis / semantic runner tool.
func IsAllowedSemanticRunner(cmdStr string) bool {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return false
	}

	// 1. Strictly forbid shell metacharacters and control operators
	if strings.ContainsAny(trimmed, ";|&`$><\n\r()") {
		return false
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}

	rawTool := fields[0]
	// 2. Forbid relative paths, /tmp paths, or arbitrary directories
	if strings.Contains(rawTool, "/") || strings.Contains(rawTool, "\\") {
		cleanPath := filepath.Clean(rawTool)
		if strings.HasPrefix(cleanPath, "/tmp/") || strings.HasPrefix(cleanPath, "/var/") ||
			strings.HasPrefix(cleanPath, "./") || strings.HasPrefix(cleanPath, "../") ||
			(!strings.HasPrefix(cleanPath, "/usr/bin/") && !strings.HasPrefix(cleanPath, "/usr/local/bin/") && !strings.HasPrefix(cleanPath, "/opt/homebrew/bin/")) {
			return false
		}
	}

	tool := strings.ToLower(filepath.Base(rawTool))
	// 3. Reject trivial / bypass tools
	if tool == "true" || tool == "false" || tool == "exit" || tool == "cat" || tool == "echo" ||
		tool == ":" || tool == "sh" || tool == "bash" || tool == "zsh" || tool == "printf" || tool == "tee" {
		return false
	}

	// 4. Reject help/version-only invocations and dummy config bypasses that disable rules or analyze nothing
	isNoopFlagOnly := true
	for _, arg := range fields[1:] {
		argLower := strings.ToLower(arg)
		if argLower == "--version" || argLower == "-v" || argLower == "-V" || argLower == "--help" || argLower == "-h" {
			continue
		}
		if argLower == "./nothing/..." || argLower == "nothing.js" || argLower == "/dev/null" || argLower == "--no-eslintrc" || argLower == "{}" || argLower == "--rule" {
			return false
		}
		isNoopFlagOnly = false
	}
	if isNoopFlagOnly && len(fields) > 1 {
		return false
	}

	trimmedLower := strings.ToLower(trimmed)
	if strings.Contains(trimmedLower, "--no-eslintrc") || strings.Contains(trimmedLower, "--rule {}") || strings.Contains(trimmedLower, "-c /dev/null") || strings.Contains(trimmedLower, "nothing.js") || strings.Contains(trimmedLower, "/dev/null") {
		return false
	}

	pol := Active()
	allowed := append(defaultAllowedSemanticRunners, pol.Reviewer.AllowedSemanticRunners...)
	for _, a := range allowed {
		aLower := strings.ToLower(strings.TrimSpace(a))
		if strings.Contains(aLower, " ") {
			if strings.HasPrefix(strings.ToLower(trimmed), aLower) {
				rest := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(trimmed), aLower))
				if rest != "" && rest != "./nothing/..." && !strings.HasPrefix(rest, "--version") && !strings.HasPrefix(rest, "--help") {
					return true
				}
			}
		} else if tool == aLower {
			return true
		}
	}
	return false
}



// DriverTimeoutConfig defines per-phase timeouts for a specific build driver.
type DriverTimeoutConfig struct {
	Compile time.Duration `json:"compile,omitempty"`
	Test    time.Duration `json:"test,omitempty"`
	Lint    time.Duration `json:"lint,omitempty"`
}

// TimeoutsConfig defines driver and job timeouts.
type TimeoutsConfig struct {
	JobTimeout time.Duration       `json:"jobTimeout,omitempty"`
	Gradle     DriverTimeoutConfig `json:"gradle,omitempty"`
	Go         DriverTimeoutConfig `json:"go,omitempty"`
	Cargo      DriverTimeoutConfig `json:"cargo,omitempty"`
	Npm        DriverTimeoutConfig `json:"npm,omitempty"`
	SwiftPM    DriverTimeoutConfig `json:"swiftpm,omitempty"`
	Xcode      DriverTimeoutConfig `json:"xcode,omitempty"`
}

// Policy specifies security, autonomy, audit, and resource constraints for Artix.
type Policy struct {
	EnterpriseMode          bool                 `json:"enterpriseMode"`
	AllowAutonomous         bool                 `json:"allowAutonomous"`
	RequireSignedPolicy     bool                 `json:"requireSignedPolicy,omitempty"`
	RequireSeparateApprover bool                 `json:"requireSeparateApprover,omitempty"`
	AllowedApprovers        []string             `json:"allowedApprovers,omitempty"`
	AuditLogPath            string               `json:"auditLogPath,omitempty"`
	AuditSigningKey         string               `json:"auditSigningKey,omitempty"`
	AuditPublicKey          string               `json:"auditPublicKey,omitempty"`
	AuditPrivateKeyPath     string               `json:"auditPrivateKeyPath,omitempty"`
	AuditRemoteSinks        []RemoteSinkConfig   `json:"auditRemoteSinks,omitempty"`
	Budget                  BudgetConfig         `json:"budget,omitempty"`
	Timeouts                TimeoutsConfig       `json:"timeouts,omitempty"`
	Reviewer                ReviewerPolicyConfig `json:"reviewer,omitempty"`
	ModelFamilies           map[string]string    `json:"modelFamilies,omitempty"`
	ModelDerivations        map[string]string    `json:"modelDerivations,omitempty"`
	AllowedTestCommands     []string             `json:"allowedTestCommands,omitempty"`
	RequireSignedTestCommands bool               `json:"requireSignedTestCommands,omitempty"`
	RequireForgeApproval    bool                 `json:"requireForgeApproval,omitempty"`
	ExpiresAt               string               `json:"expiresAt,omitempty"`
	ApproverIdentity        string               `json:"approverIdentity,omitempty"`
	PolicyHash                string               `json:"policyHash,omitempty"`
	AllowedDrivers            []string             `json:"allowedDrivers,omitempty"`
	Pilot                     PilotPolicyConfig    `json:"pilot,omitempty"`
	Source                  string               `json:"-"`
	IsVerified              bool                 `json:"-"`
}

// PilotPolicyConfig defines pilot stop rule thresholds.
type PilotPolicyConfig struct {
	MaxAbortRate    float64 `json:"maxAbortRate,omitempty"`
	MaxCostPerStory float64 `json:"maxCostPerStory,omitempty"`
}

// GetAllowedDrivers returns the list of allowed build drivers. Defaults to ["go", "gradle"].
func (p *Policy) GetAllowedDrivers() []string {
	if p == nil || len(p.AllowedDrivers) == 0 {
		return []string{"go", "gradle"}
	}
	return p.AllowedDrivers
}

// IsDriverAllowed checks if a driver is permitted under the policy.
func (p *Policy) IsDriverAllowed(driver string) bool {
	driver = strings.ToLower(strings.TrimSpace(driver))
	allowed := p.GetAllowedDrivers()
	for _, a := range allowed {
		if strings.ToLower(strings.TrimSpace(a)) == driver {
			return true
		}
	}
	return false
}

// ValidateDriverAllowed returns an error if the driver is not permitted under the policy.
func (p *Policy) ValidateDriverAllowed(driver string) error {
	if !p.IsDriverAllowed(driver) {
		return fmt.Errorf("build driver %q is not allowed by policy; allowedDrivers: %v", driver, p.GetAllowedDrivers())
	}
	return nil
}

// GetDriverTimeout returns the configured or default timeout for a driver and phase.
func (p *Policy) GetDriverTimeout(driver, phase string) time.Duration {
	var dt DriverTimeoutConfig
	if p != nil {
		switch strings.ToLower(driver) {
		case "gradle":
			dt = p.Timeouts.Gradle
		case "go":
			dt = p.Timeouts.Go
		case "cargo":
			dt = p.Timeouts.Cargo
		case "npm":
			dt = p.Timeouts.Npm
		case "swiftpm", "swift":
			dt = p.Timeouts.SwiftPM
		case "xcode":
			dt = p.Timeouts.Xcode
		}
	}
	switch strings.ToLower(phase) {
	case "compile":
		if dt.Compile > 0 {
			return dt.Compile
		}
		if strings.EqualFold(driver, "gradle") || strings.EqualFold(driver, "xcode") {
			return 300 * time.Second
		}
		return 180 * time.Second
	case "test":
		if dt.Test > 0 {
			return dt.Test
		}
		if strings.EqualFold(driver, "gradle") || strings.EqualFold(driver, "xcode") {
			return 360 * time.Second
		}
		return 300 * time.Second
	case "lint":
		if dt.Lint > 0 {
			return dt.Lint
		}
		if strings.EqualFold(driver, "gradle") || strings.EqualFold(driver, "cargo") {
			return 180 * time.Second
		}
		return 120 * time.Second
	default:
		return 180 * time.Second
	}
}

// GetTimeout retrieves the timeout for a given driver and phase from the active policy.
func GetTimeout(driver, phase string) time.Duration {
	return Active().GetDriverTimeout(driver, phase)
}

// IsDisjointModelFamiliesEnforced returns true if disjoint coder and critic model families are mandated.
// It defaults to true in enterprise mode.
func (p *Policy) IsDisjointModelFamiliesEnforced() bool {
	if p == nil {
		return false
	}
	if p.EnterpriseMode || IsEnterprise() {
		return true
	}
	return p.Reviewer.EnforceDisjointModelFamilies
}

var (
	policyMu     sync.RWMutex
	cachedPolicy *Policy
)

// SetDefaultPolicyPath overrides the default policy path (used for testing or build injection).
func SetDefaultPolicyPath(p string) {
	policyMu.Lock()
	defer policyMu.Unlock()
	DefaultPolicyPath = p
	cachedPolicy = nil
}

// ResetCache clears the cached policy.
func ResetCache() {
	policyMu.Lock()
	defer policyMu.Unlock()
	cachedPolicy = nil
}

// LoadPolicy reads and verifies policy from path.
// It enforces that the policy file must be root-owned (UID 0) or cryptographically signed.
func LoadPolicy(path string) (*Policy, error) {
	if path == "" {
		if envPath := os.Getenv("ARTIX_POLICY_PATH"); envPath != "" {
			path = envPath
		}
		if path == "" {
			path = DefaultPolicyPath
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("policy file not found at %s: %w", path, err)
	}

	// Verify root-owned or signature
	isRoot := isFileRootOwned(info)
	sigValid, errSig := verifyPolicySignature(path)

	if isSignedPolicyEnforced() {
		if !sigValid {
			return nil, fmt.Errorf("policy file at %s is untrusted: signed-policy enforcement active; policy must be cryptographically signed (sig error: %v)", path, errSig)
		}
	} else {
		if !isRoot && !sigValid {
			return nil, fmt.Errorf("policy file at %s is untrusted: must be root-owned (UID 0) or cryptographically signed (sig error: %v)", path, errSig)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}

	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse policy JSON: %w", err)
	}

	h := sha256.Sum256(data)
	p.PolicyHash = hex.EncodeToString(h[:])
	p.Source = path
	p.IsVerified = true
	return &p, nil
}

func isFileRootOwned(info os.FileInfo) bool {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Uid == 0
	}
	return false
}

func verifyPolicySignature(policyPath string) (bool, error) {
	sigPath := policyPath + ".sig"
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		return false, fmt.Errorf("signature file %s not found: %w", sigPath, err)
	}

	content, err := os.ReadFile(policyPath)
	if err != nil {
		return false, err
	}

	trustedKeysMu.RLock()
	defer trustedKeysMu.RUnlock()

	trimmedSig := strings.TrimSpace(string(sigData))

	// 1. Asymmetric Ed25519 Verification (Enterprise Trust Root)
	if sigBytes, err := hex.DecodeString(trimmedSig); err == nil && len(sigBytes) == ed25519.SignatureSize {
		for _, pubKey := range trustedPublicKeys {
			if ed25519.Verify(pubKey, content, sigBytes) {
				return true, nil
			}
		}

		// Also check ARTIX_POLICY_TRUSTED_PUBKEY env var ONLY if enterprise ldflag is NOT enforced
		if !isSignedPolicyEnforced() {
			if envPubHex := os.Getenv("ARTIX_POLICY_TRUSTED_PUBKEY"); envPubHex != "" {
				if rawPub, err := hex.DecodeString(strings.TrimSpace(envPubHex)); err == nil && len(rawPub) == ed25519.PublicKeySize {
					if ed25519.Verify(ed25519.PublicKey(rawPub), content, sigBytes) {
						return true, nil
					}
				}
			}
		}
	}

	// 2. Symmetric HMAC Verification (Fallback) - Only for non-enterprise binaries
	if !isSignedPolicyEnforced() && len(trustedKeys) == 0 {
		// Check ARTIX_POLICY_SIGNING_KEY in environment for verification if available
		if k := os.Getenv("ARTIX_POLICY_SIGNING_KEY"); k != "" {
			mac := hmac.New(sha256.New, []byte(k))
			mac.Write(content)
			expected := hex.EncodeToString(mac.Sum(nil))
			if hmac.Equal([]byte(expected), []byte(trimmedSig)) {
				return true, nil
			}
		}
		if len(trustedPublicKeys) > 0 {
			return false, errors.New("policy signature does not match any trusted ed25519 public key")
		}
		return false, errors.New("no trusted signing keys registered to verify policy signature")
	}

	if !isSignedPolicyEnforced() {
		for _, secret := range trustedKeys {
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(content)
			expected := hex.EncodeToString(mac.Sum(nil))
			if hmac.Equal([]byte(expected), []byte(trimmedSig)) {
				return true, nil
			}
		}
	}

	return false, errors.New("policy signature does not match any trusted key")
}

// SignPolicyFile signs a policy file at policyPath using the given symmetric secret and writes policyPath.sig.
func SignPolicyFile(policyPath string, secret string) error {
	content, err := os.ReadFile(policyPath)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(content)
	sig := hex.EncodeToString(mac.Sum(nil))
	return os.WriteFile(policyPath+".sig", []byte(sig), 0644)
}

// SignPolicyFileEd25519 signs a policy file at policyPath using an asymmetric Ed25519 private key.
func SignPolicyFileEd25519(policyPath string, privKey ed25519.PrivateKey) error {
	content, err := os.ReadFile(policyPath)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(privKey, content)
	sigHex := hex.EncodeToString(sig)
	return os.WriteFile(policyPath+".sig", []byte(sigHex), 0644)
}

// RequireSignedPolicyLdflag can be set at compile time via -ldflags "-X artix/pkg/policy.RequireSignedPolicyLdflag=true"
var RequireSignedPolicyLdflag = "false"

// RequireSignedPolicyFlag can be set at compile time via -ldflags "-X artix/pkg/policy.RequireSignedPolicyFlag=true"
// or programmatically via EnforceSignedPolicy.
var RequireSignedPolicyFlag = "false"

// IsSignedPolicyEnforced returns whether enterprise signed policy enforcement is active via compile-time flag.
func IsSignedPolicyEnforced() bool {
	return isSignedPolicyEnforced()
}

func isSignedPolicyEnforced() bool {
	flagBool := strings.EqualFold(RequireSignedPolicyFlag, "true") || RequireSignedPolicyFlag == "1"
	ldflagBool := strings.EqualFold(RequireSignedPolicyLdflag, "true") || RequireSignedPolicyLdflag == "1"
	return flagBool || ldflagBool
}

// EnforceSignedPolicy permanently mandates that policy evaluation requires a verified cryptographic signature.
func EnforceSignedPolicy() {
	RequireSignedPolicyFlag = "true"
}

// ResetCachedPolicy clears the cached policy so tests or reloads can re-evaluate fresh policy state.
func ResetCachedPolicy() {
	policyMu.Lock()
	defer policyMu.Unlock()
	cachedPolicy = nil
}

// SetActivePolicyForTest sets an in-memory policy for testing purposes.
func SetActivePolicyForTest(p *Policy) {
	policyMu.Lock()
	defer policyMu.Unlock()
	if p != nil {
		p.IsVerified = true
	}
	cachedPolicy = p
}

// ResetTestPolicy clears any test policy override.
func ResetTestPolicy() {
	ResetCachedPolicy()
}

// Active returns the currently active policy. If a verified system policy file exists,
// it caches and returns it. If unverified, it fails closed in enterprise mode or evaluates
// environment dynamically without poisoning the cache.
func Active() *Policy {
	policyMu.RLock()
	if cachedPolicy != nil && cachedPolicy.IsVerified {
		p := *cachedPolicy
		policyMu.RUnlock()
		return &p
	}
	policyMu.RUnlock()

	policyMu.Lock()
	defer policyMu.Unlock()

	if cachedPolicy != nil && cachedPolicy.IsVerified {
		p := *cachedPolicy
		return &p
	}

	policyPath := DefaultPolicyPath
	if envPath := os.Getenv("ARTIX_POLICY_PATH"); envPath != "" {
		policyPath = envPath
	}
	p, err := LoadPolicy(policyPath)
	if err == nil && p != nil && p.IsVerified {
		cachedPolicy = p
		res := *cachedPolicy
		return &res
	}

	// Fallback when no system policy file is present or verification failed
	isEnvEnterprise := os.Getenv("ARTIX_ENTERPRISE") == "1" || os.Getenv("KRITIX_ENTERPRISE") == "1"
	return &Policy{
		EnterpriseMode:          isEnvEnterprise || isSignedPolicyEnforced(),
		AllowAutonomous:         false, // FAIL CLOSED: unsigned or missing policy cannot grant autonomy
		RequireSignedPolicy:     true,
		RequireSeparateApprover: isEnvEnterprise || isSignedPolicyEnforced(),
		Source:                  "fail_closed_unverified",
		IsVerified:              false,
	}
}

// VerifyAutonomyGate validates that autonomous operations are permitted by a cryptographically signed, unexpired policy.
// It fails closed if the signature is absent, invalid, or expired.
func VerifyAutonomyGate() error {
	pol := Active()
	if pol == nil {
		return errors.New("autonomy gate failed: no policy loaded")
	}
	if !pol.IsVerified {
		return errors.New("autonomy gate failed: signed policy is absent, invalid, or unverified (fail closed)")
	}
	if !pol.AllowAutonomous {
		return errors.New("autonomy gate failed: allowAutonomous is not enabled in signed policy")
	}
	if pol.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, pol.ExpiresAt)
		if err != nil {
			exp, err = time.Parse(time.RFC3339Nano, pol.ExpiresAt)
		}
		if err != nil {
			return fmt.Errorf("autonomy gate failed: invalid expiresAt timestamp %q: %w", pol.ExpiresAt, err)
		}
		if time.Now().UTC().After(exp) {
			return fmt.Errorf("autonomy gate failed: signed policy expired at %s", pol.ExpiresAt)
		}
	}
	return nil
}

// GetPolicyHash returns the SHA-256 hash of the active signed policy.
func GetPolicyHash() string {
	pol := Active()
	if pol != nil {
		return pol.PolicyHash
	}
	return ""
}

// IsEnterprise returns true if enterprise mode is enforced by verified policy or environment.
// If a verified enterprise policy is loaded with EnterpriseMode=true, environment variables CANNOT loosen it.
func IsEnterprise() bool {
	pol := Active()
	if pol.IsVerified && pol.EnterpriseMode {
		return true
	}
	return os.Getenv("ARTIX_ENTERPRISE") == "1" || os.Getenv("KRITIX_ENTERPRISE") == "1" || isSignedPolicyEnforced()
}

// IsAutonomousAllowed returns whether autonomous commits are permitted by the signed autonomy gate.
func IsAutonomousAllowed() bool {
	return VerifyAutonomyGate() == nil
}

// ValidateApprover enforces Separation of Duties (Four-Eyes Principle).
// In enterprise mode or when RequireSeparateApprover is set, autonomous merges and PRs
// must record an approver identity that is non-empty and strictly distinct from the bot's identity.
func ValidateApprover(botIdentity, approverIdentity string) error {
	pol := Active()
	requiresSeparation := pol.RequireSeparateApprover || IsEnterprise()
	if !requiresSeparation {
		return nil
	}

	trimmedApprover := strings.TrimSpace(approverIdentity)
	if trimmedApprover == "" {
		return errors.New("separation of duties violation: approver identity is required for autonomous operations")
	}

	botLower := strings.ToLower(strings.TrimSpace(botIdentity))
	approverLower := strings.ToLower(trimmedApprover)

	// Block self-approval by bot
	if (botLower != "" && approverLower == botLower) ||
		approverLower == "artix-agent" ||
		approverLower == "artix-bot" ||
		approverLower == "bot" ||
		strings.Contains(approverLower, "[bot]") {
		return fmt.Errorf("separation of duties violation: bot identity %q cannot approve its own autonomous merge", trimmedApprover)
	}

	// If AllowedApprovers list is configured, enforce membership
	if len(pol.AllowedApprovers) > 0 {
		found := false
		for _, allowed := range pol.AllowedApprovers {
			if strings.EqualFold(allowed, trimmedApprover) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("policy violation: approver %q is not authorized in allowedApprovers list", trimmedApprover)
		}
	}

	return nil
}

// PRApproval represents a verified pull request review record from GitHub or GitLab API.
type PRApproval struct {
	ApproverUsername string `json:"approverUsername"`
	AuthorUsername   string `json:"authorUsername"`
	State            string `json:"state"` // Must be "APPROVED"
	CommitSHA        string `json:"commitSha,omitempty"`
	Signature        string `json:"signature,omitempty"`
	Source           string `json:"source,omitempty"` // "github_api_server_verified", "gitlab_api_server_verified", "oidc"
	VerifiedByForge  bool   `json:"verifiedByForge"`
}

// VerifyForgeToken verifies that the PRApproval has a valid, unforgeable cryptographic signature.
func VerifyForgeToken(a *PRApproval) bool {
	if a == nil || strings.TrimSpace(a.Signature) == "" {
		return false
	}
	return forgesec.VerifyToken(a.ApproverUsername, a.AuthorUsername, a.State, a.CommitSHA, a.Source, a.Signature)
}

// ValidateForgeApproval enforces Separation of Duties using a verified PR review from the forge.
// The approver must be an approved human reviewer, strictly distinct from the PR author and any bot identity.
func ValidateForgeApproval(approval *PRApproval, authorIdentity, botIdentity string) error {
	pol := Active()
	requiresSeparation := pol.RequireSeparateApprover || pol.RequireForgeApproval || IsEnterprise()
	if !requiresSeparation {
		return nil
	}

	if approval == nil {
		return errors.New("separation of duties violation: verified forge PR approval record is required")
	}

	if !VerifyForgeToken(approval) {
		return errors.New("separation of duties violation: forged or unverified approval signature; approvals must be minted server-side by forge package")
	}

	trimmedApprover := strings.TrimSpace(approval.ApproverUsername)
	if trimmedApprover == "" {
		return errors.New("separation of duties violation: approver username in forge approval record is empty")
	}

	if !strings.EqualFold(approval.State, "APPROVED") {
		return fmt.Errorf("separation of duties violation: forge review status is %q, expected APPROVED", approval.State)
	}

	approverLower := strings.ToLower(trimmedApprover)
	botLower := strings.ToLower(strings.TrimSpace(botIdentity))
	authorLower := strings.ToLower(strings.TrimSpace(authorIdentity))
	recordAuthorLower := strings.ToLower(strings.TrimSpace(approval.AuthorUsername))

	// Self-approval checks
	if recordAuthorLower != "" && approverLower == recordAuthorLower {
		return fmt.Errorf("separation of duties violation: PR author %q cannot approve their own PR", approval.AuthorUsername)
	}
	if authorLower != "" && approverLower == authorLower {
		return fmt.Errorf("separation of duties violation: PR author %q cannot approve their own PR", authorIdentity)
	}

	// Bot denylist checks
	if (botLower != "" && approverLower == botLower) ||
		approverLower == "artix-agent" ||
		approverLower == "artix-bot" ||
		approverLower == "bot" ||
		strings.Contains(approverLower, "[bot]") ||
		strings.HasSuffix(approverLower, "-bot") {
		return fmt.Errorf("separation of duties violation: bot identity %q cannot approve autonomous merge", trimmedApprover)
	}

	// Allowed approvers membership check
	if len(pol.AllowedApprovers) > 0 {
		found := false
		for _, allowed := range pol.AllowedApprovers {
			if strings.EqualFold(allowed, trimmedApprover) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("policy violation: forge approver %q is not authorized in allowedApprovers list", trimmedApprover)
		}
	}

	return nil
}

// ValidateTestCommands validates test commands against the signed enterprise policy.
// In enterprise mode or when RequireSignedTestCommands is active, unapproved test commands
// from untrusted specs or repositories are strictly rejected.
func ValidateTestCommands(commands []string) ([]string, error) {
	pol := Active()
	if !IsEnterprise() && !pol.RequireSignedTestCommands {
		return commands, nil
	}

	if len(pol.AllowedTestCommands) == 0 {
		return nil, errors.New("enterprise policy violation: allowedTestCommands list must be configured in signed policy for enterprise execution")
	}

	var verified []string
	for _, cmd := range commands {
		cmdTrimmed := strings.TrimSpace(cmd)
		if cmdTrimmed == "" {
			continue
		}
		allowed := false
		for _, allowedCmd := range pol.AllowedTestCommands {
			if cmdTrimmed == strings.TrimSpace(allowedCmd) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("enterprise policy violation: test command %q is not authorized in signed policy allowedTestCommands %v", cmd, pol.AllowedTestCommands)
		}
		verified = append(verified, cmdTrimmed)
	}
	return verified, nil
}

// EffectiveAuditLogPath returns the audit log destination.
// In Enterprise Mode, ARTIX_AUDIT_LOG may redirect the log path but may NOT disable it (e.g. /dev/null or empty/none).
func EffectiveAuditLogPath(workspaceDir string) string {
	pol := Active()
	if IsEnterprise() {
		if pol.IsVerified && pol.AuditLogPath != "" {
			return pol.AuditLogPath
		}
		if env := os.Getenv("ARTIX_AUDIT_LOG"); env != "" {
			trimmed := strings.TrimSpace(env)
			lower := strings.ToLower(trimmed)
			if lower != "/dev/null" && lower != "none" && lower != "off" && lower != "disabled" && lower != "false" && lower != "0" {
				return trimmed
			}
		}
		if workspaceDir != "" {
			return filepath.Join(workspaceDir, ".artix", "audit.jsonl")
		}
		return ".artix/audit.jsonl"
	}

	if env := os.Getenv("ARTIX_AUDIT_LOG"); env != "" {
		return env
	}

	if workspaceDir != "" {
		return filepath.Join(workspaceDir, ".artix", "audit.jsonl")
	}
	return ".artix/audit.jsonl"
}
