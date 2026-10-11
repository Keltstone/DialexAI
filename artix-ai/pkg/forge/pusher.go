package forge

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"artix/pkg/audit"
	"artix/pkg/git"
	"artix/pkg/policy"
)

// IsProtectedBranch returns true if the target branch is a protected/default repository branch.
func IsProtectedBranch(branch string) bool {
	b := strings.ToLower(strings.TrimSpace(branch))
	b = strings.TrimPrefix(b, "refs/heads/")
	b = strings.TrimPrefix(b, "origin/")

	// Exact matches for standard protected/default branches
	if b == "main" || b == "master" || b == "trunk" || b == "develop" || b == "dev" || b == "staging" || b == "prod" || b == "production" {
		return true
	}
	// Prefix matches for standard protected branch hierarchies
	if strings.HasPrefix(b, "release/") || strings.HasPrefix(b, "releases/") || strings.HasPrefix(b, "hotfix/") || strings.HasPrefix(b, "hotfixes/") {
		return true
	}
	return false
}

// IsDefinitiveRejection returns true only if the error represents an explicit human/policy rejection
// from an authorized reviewer (e.g., changes requested, dismissed review),
// and false for non-definitive states (bot reviews, unauthorized reviews, no reviews yet, 5xx server errors, network timeouts).
func IsDefinitiveRejection(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrChangesRequested) ||
		errors.Is(err, ErrReviewDismissed) {
		return true
	}
	return false
}

// Phase1Binding holds verified Phase 1 candidate metadata.
type Phase1Binding struct {
	StorySpecID  string
	CandidateSHA string
	VerdictHash  string
	PRBranch     string
}

// verifyPhase1AuditBinding asserts that the candidate SHA and spec ID are bound to a Phase 1 audit record.
func verifyPhase1AuditBinding(logPath, storyID, candidateSHA string, expectedVerdictHash ...string) (*Phase1Binding, error) {
	if strings.TrimSpace(logPath) == "" {
		return nil, fmt.Errorf("phase 1 audit log path is required (fail closed)")
	}
	if strings.TrimSpace(storyID) == "" {
		return nil, fmt.Errorf("phase 1 binding requires non-empty spec ID (fail closed)")
	}
	if strings.TrimSpace(candidateSHA) == "" {
		return nil, fmt.Errorf("phase 1 binding requires non-empty candidate SHA (fail closed)")
	}

	expVerdict := ""
	if len(expectedVerdictHash) > 0 {
		expVerdict = strings.TrimSpace(expectedVerdictHash[0])
	}

	// Cryptographic whole-log verification
	pubKeyHex := ""
	isEnterprise := policy.IsEnterprise()
	if !policy.IsSignedPolicyEnforced() {
		pubKeyHex = strings.TrimSpace(os.Getenv("ARTIX_AUDIT_PUBLIC_KEY"))
	}
	if pubKeyHex == "" {
		pubKeyHex = strings.TrimSpace(policy.Active().AuditPublicKey)
	}

	if pubKeyHex != "" {
		pubKeyBytes, decodeErr := hex.DecodeString(pubKeyHex)
		if decodeErr != nil || len(pubKeyBytes) == 0 {
			return nil, fmt.Errorf("invalid ARTIX_AUDIT_PUBLIC_KEY: %w", decodeErr)
		}
		if _, vErr := audit.VerifyLogWithPubKey(logPath, pubKeyBytes); vErr != nil {
			return nil, fmt.Errorf("phase 1 audit log cryptographic integrity verification failed: %w", vErr)
		}
	} else if isEnterprise {
		return nil, fmt.Errorf("enterprise mode requires an asymmetric audit public key for verification; unkeyed logs are strictly forbidden (fail closed)")
	} else {
		if _, vErr := audit.VerifyLog(logPath); vErr != nil {
			return nil, fmt.Errorf("phase 1 audit log cryptographic integrity verification failed: %w", vErr)
		}
	}

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("phase 1 audit log missing: no records found at %s", logPath)
		}
		return nil, fmt.Errorf("cannot read audit log: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var binding *Phase1Binding

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev audit.AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}

		evStoryID := ev.StorySpecID
		evCandidateSHA := ""
		evVerdict := ""
		evBranch := ""
		if ev.Details != nil {
			if s, ok := ev.Details["storyId"].(string); ok && s != "" {
				evStoryID = s
			}
			if c, ok := ev.Details["candidateSHA"].(string); ok && c != "" {
				evCandidateSHA = c
			} else if c, ok := ev.Details["commitHash"].(string); ok && c != "" {
				evCandidateSHA = c
			}
			if v, ok := ev.Details["reviewerVerdictHash"].(string); ok && v != "" {
				evVerdict = v
			} else if v, ok := ev.Details["verdictHash"].(string); ok && v != "" {
				evVerdict = v
			}
			if b, ok := ev.Details["prBranch"].(string); ok && b != "" {
				evBranch = b
			}
		}

		if ev.EventType == "CANDIDATE_PUSHED" &&
			(ev.Status == "AWAITING_APPROVAL" || ev.Status == "SUCCESS") &&
			evStoryID == storyID && evCandidateSHA == candidateSHA {
			if evVerdict == "" {
				return nil, fmt.Errorf("phase 1 audit record missing reviewer verdict hash (fail closed)")
			}
			if expVerdict != "" && evVerdict != expVerdict {
				return nil, fmt.Errorf("reviewer verdict hash mismatch in Phase 1 audit record: expected %s, got %s", expVerdict, evVerdict)
			}
			binding = &Phase1Binding{
				StorySpecID:  evStoryID,
				CandidateSHA: evCandidateSHA,
				VerdictHash:  evVerdict,
				PRBranch:     evBranch,
			}
			break
		}
	}

	if binding == nil {
		return nil, fmt.Errorf("candidate SHA %s is not bound to a valid verified Phase 1 CANDIDATE_PUSHED audit record for spec %s", candidateSHA, storyID)
	}
	return binding, nil
}

// NewForgePusher constructs a ForgePusher function with branch protection enforcement.
func NewForgePusher(driver *git.Driver, remote, prBranch string) func(ctx context.Context, commitSHA string) error {
	return func(ctx context.Context, commitSHA string) error {
		if IsProtectedBranch(prBranch) {
			return fmt.Errorf("branch protection violation: forbidden push directly to protected branch %q; autonomous candidates must target a dedicated PR branch", prBranch)
		}
		if driver == nil {
			return fmt.Errorf("git driver is required for ForgePusher")
		}
		refspec := fmt.Sprintf("%s:refs/heads/%s", commitSHA, prBranch)
		_, err := driver.Push(remote, refspec)
		if err != nil {
			return fmt.Errorf("failed to push candidate commit %s to %s/%s: %w", commitSHA, remote, prBranch, err)
		}
		return nil
	}
}

// CleanupCandidateBranch deletes the candidate branch on the remote forge.
func CleanupCandidateBranch(ctx context.Context, driver *git.Driver, remote, prBranch string) error {
	if IsProtectedBranch(prBranch) {
		return fmt.Errorf("refusing to delete protected branch %q", prBranch)
	}
	if driver == nil {
		return fmt.Errorf("git driver required")
	}
	_, err := driver.DeleteRemoteBranch(remote, prBranch)
	return err
}

// VerifyAndMergeCandidate executes Phase 2 of the autonomous flow: verifies server-side forge approval
// on the exact candidate commit SHA and commits audit records.
func VerifyAndMergeCandidate(ctx context.Context, driver *git.Driver, verifier func(ctx context.Context, commitSHA string) (*policy.PRApproval, error), candidateSHA string, auditLogger *audit.Logger, storyID, remote, prBranch string, expectedVerdictHash ...string) (*policy.PRApproval, error) {
	if verifier == nil {
		return nil, fmt.Errorf("verifier is required for Phase 2 forge verification")
	}
	if strings.TrimSpace(storyID) == "" {
		return nil, fmt.Errorf("phase 2 verification error: --spec is required and cannot be empty (fail closed)")
	}

	workDir := ""
	if driver != nil {
		workDir = driver.WorkDir()
	}
	logPath := ""
	if auditLogger != nil {
		logPath = auditLogger.LogPath()
	} else if workDir != "" {
		logPath = policy.EffectiveAuditLogPath(workDir)
	}

	expVerdict := ""
	if len(expectedVerdictHash) > 0 {
		expVerdict = strings.TrimSpace(expectedVerdictHash[0])
	}

	isEnterprise := policy.IsEnterprise()

	// When audit logger is provided or audit log exists or enterprise mode is active, Phase 1 binding MUST be verified
	var binding *Phase1Binding
	if logPath != "" {
		if _, statErr := os.Stat(logPath); statErr == nil || isEnterprise || auditLogger != nil {
			var bErr error
			binding, bErr = verifyPhase1AuditBinding(logPath, storyID, candidateSHA, expVerdict)
			if bErr != nil {
				return nil, fmt.Errorf("phase 2 verification error: %w", bErr)
			}
		}
	} else if isEnterprise || auditLogger != nil {
		return nil, fmt.Errorf("phase 2 verification error: Phase 1 audit binding verification required (fail closed)")
	}

	actualPRBranch := prBranch
	if binding != nil && binding.PRBranch != "" {
		actualPRBranch = binding.PRBranch
	}

	approval, err := verifier(ctx, candidateSHA)
	if err != nil {
		// Clean up remote candidate branch ONLY on definitive negative reviews from authorized humans
		if IsDefinitiveRejection(err) && driver != nil && remote != "" && actualPRBranch != "" {
			_ = CleanupCandidateBranch(ctx, driver, remote, actualPRBranch)
		}
		return nil, fmt.Errorf("phase 2 forge approval verification failed: %w", err)
	}

	if approval == nil || !approval.VerifiedByForge {
		sepErr := fmt.Errorf("separation of duties violation: %w", ErrForgedApproval)
		if IsDefinitiveRejection(sepErr) && driver != nil && remote != "" && actualPRBranch != "" {
			_ = CleanupCandidateBranch(ctx, driver, remote, actualPRBranch)
		}
		return nil, sepErr
	}

	if err := policy.ValidateForgeApproval(approval, "artix-agent", "artix-agent"); err != nil {
		if IsDefinitiveRejection(err) && driver != nil && remote != "" && actualPRBranch != "" {
			_ = CleanupCandidateBranch(ctx, driver, remote, actualPRBranch)
		}
		return nil, fmt.Errorf("forge approval validation failed: %w", err)
	}

	var emitErr error
	if auditLogger != nil {
		emitErr = auditLogger.Emit(audit.AuditEvent{
			EventType: audit.EventCodeConvergence,
			Status:    "APPROVAL_VERIFIED",
			Approver:  approval.ApproverUsername,
			Details: map[string]any{
				"storyId":    storyID,
				"commitHash": candidateSHA,
				"prBranch":   actualPRBranch,
				"state":      "APPROVAL_VERIFIED",
			},
		})
	} else if workDir != "" {
		emitErr = audit.Default(workDir).Emit(audit.AuditEvent{
			EventType: audit.EventCodeConvergence,
			Status:    "APPROVAL_VERIFIED",
			Approver:  approval.ApproverUsername,
			Details: map[string]any{
				"storyId":    storyID,
				"commitHash": candidateSHA,
				"prBranch":   actualPRBranch,
				"state":      "APPROVAL_VERIFIED",
			},
		})
	}
	if emitErr != nil {
		return nil, fmt.Errorf("phase 2 APPROVAL_VERIFIED audit emission failed: %w", emitErr)
	}

	return approval, nil
}



