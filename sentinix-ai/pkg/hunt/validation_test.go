package hunt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

func TestValidationStage1SchemaFilter(t *testing.T) {
	vp := NewValidationPipeline(&HuntConfig{
		Validation: ValidationConfig{
			MinConfidence: 0.70,
		},
	}, "", nil)

	candidates := []CandidateFinding{
		{
			Finding:    engines.Finding{Rule: "hunt/sql-injection", Message: "Unsanitized user input in query", Severity: engines.High},
			Confidence: 0.85,
		},
		{
			Finding:    engines.Finding{Rule: "hunt/authz", Message: "Low confidence finding", Severity: engines.Medium},
			Confidence: 0.50, // Should be dropped (< 0.70)
		},
		{
			Finding:    engines.Finding{Rule: "hunt/path-traversal", Message: "   ", Severity: engines.High}, // Empty reason
			Confidence: 0.90,
		},
	}

	filtered := vp.FilterSchemaAndConfidence(candidates)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 finding to pass Stage 1, got %d", len(filtered))
	}
	if filtered[0].Finding.Rule != "hunt/sql-injection" {
		t.Errorf("unexpected finding passed: %s", filtered[0].Finding.Rule)
	}
}

func TestValidationStage2Mitigations(t *testing.T) {
	tmp := t.TempDir()

	manifest := `<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".PrivateActivity" android:exported="false"/>
        <activity android:name=".PublicActivity" android:exported="true"/>
    </application>
</manifest>`
	if err := os.WriteFile(filepath.Join(tmp, "AndroidManifest.xml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	vp := NewValidationPipeline(&HuntConfig{
		Validation: ValidationConfig{
			MitigationCheck: true,
		},
	}, tmp, nil)

	candidates := []CandidateFinding{
		{
			Finding:    engines.Finding{Rule: "hunt/confused-deputy", File: "PrivateActivity.java", Severity: engines.High, Message: "intent redirection"},
			Confidence: 0.90,
		},
		{
			Finding:    engines.Finding{Rule: "hunt/confused-deputy", File: "PublicActivity.java", Severity: engines.High, Message: "intent redirection"},
			Confidence: 0.90,
		},
	}

	mitigated := vp.VerifyMitigations(candidates)
	if len(mitigated) != 2 {
		t.Fatalf("expected 2 findings returned, got %d", len(mitigated))
	}

	// PrivateActivity should be marked mitigated with severity Demoted to Info
	if !mitigated[0].Mitigated || mitigated[0].Finding.Severity != engines.Info {
		t.Errorf("PrivateActivity was not mitigated: %+v", mitigated[0])
	}
	if !strings.Contains(mitigated[0].Finding.Message, "android:exported=\"false\"") {
		t.Errorf("expected mitigation reason in message, got: %s", mitigated[0].Finding.Message)
	}

	// PublicActivity should remain unmitigated High
	if mitigated[1].Mitigated || mitigated[1].Finding.Severity != engines.High {
		t.Errorf("PublicActivity should not be mitigated: %+v", mitigated[1])
	}
}

func TestValidationStage4Calibration(t *testing.T) {
	vp := NewValidationPipeline(&HuntConfig{
		Validation: ValidationConfig{
			SeverityCalibration: true,
		},
	}, "", nil)

	candidates := []CandidateFinding{
		{
			Finding:        engines.Finding{Rule: "hunt/idor", Severity: engines.Critical, Message: "idor without proof"},
			Exploitability: "unconfirmed",
			Confidence:     0.75,
		},
		{
			Finding:        engines.Finding{Rule: "hunt/sql-injection", Severity: engines.Critical, Message: "sqli with proof"},
			Exploitability: "confirmed",
			Confidence:     0.95,
		},
	}

	calibrated := vp.CalibrateSeverity(candidates)
	if len(calibrated) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(calibrated))
	}

	// Unconfirmed critical should be capped at High
	if calibrated[0].Finding.Severity != engines.High {
		t.Errorf("expected unconfirmed finding to be capped at High, got %s", calibrated[0].Finding.Severity)
	}

	// Confirmed critical should stay Critical and get CVSS bonus
	if calibrated[1].Finding.Severity != engines.Critical || calibrated[1].CVSSScore < 9.5 {
		t.Errorf("expected confirmed finding to stay Critical with high CVSS, got sev=%s cvss=%f",
			calibrated[1].Finding.Severity, calibrated[1].CVSSScore)
	}
}

func TestValidationStage5Deduplication(t *testing.T) {
	vp := NewValidationPipeline(&HuntConfig{
		Validation: ValidationConfig{
			Dedupe: true,
		},
	}, "", nil)

	candidates := []CandidateFinding{
		{
			Finding:        engines.Finding{Rule: "hunt/sql-injection", File: "api.go", Match: "getUser:sql-injection"},
			Confidence:     0.80,
			Exploitability: "unconfirmed",
		},
		{
			Finding:        engines.Finding{Rule: "hunt/sql-injection", File: "api.go", Match: "getUser:sql-injection"},
			Confidence:     0.92,
			Exploitability: "confirmed", // Should replace the unconfirmed one
		},
	}

	deduped := vp.Deduplicate(candidates)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated finding, got %d", len(deduped))
	}
	if deduped[0].Exploitability != "confirmed" || deduped[0].Confidence != 0.92 {
		t.Errorf("deduplication did not retain highest exploitability finding: %+v", deduped[0])
	}
}

func TestValidationFPMemory(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Seed findings with low precision for rule "hunt/noisy-rule"
	now := time.Now().Unix()
	for i := 0; i < 10; i++ {
		status := "false_positive"
		if i == 0 {
			status = "open" // 1 out of 10 = 10% precision
		}
		_, err = st.DB().Exec(
			"insert into findings(id, project_id, fingerprint, rule, engine, file, line, severity, message, evidence, cwe, fix, status, owner, first_commit, first_seen, last_seen, updated, accepted_reason, accepted_until, ticket, source, category, pr) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)",
			fmt.Sprintf("f-%d", i), "proj-1", fmt.Sprintf("fp-%d", i), "hunt/noisy-rule", "llm-audit", "test.go", 1, 3, "msg", "", "", "", status, "", "", now, now, now, "", 0, "", "llm", "security", "",
		)
		if err != nil {
			t.Fatalf("failed to insert finding: %v", err)
		}
	}

	vp := NewValidationPipeline(&HuntConfig{}, "", st)

	candidates := []CandidateFinding{
		{
			Finding: engines.Finding{Rule: "hunt/noisy-rule", Severity: engines.High, Message: "noisy finding"},
		},
		{
			Finding: engines.Finding{Rule: "hunt/clean-rule", Severity: engines.High, Message: "clean finding"},
		},
	}

	applied := vp.ApplyFPMemory(candidates)
	if len(applied) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(applied))
	}

	if applied[0].Finding.Severity != engines.Info {
		t.Errorf("expected noisy-rule to be demoted to Info, got: %s", applied[0].Finding.Severity)
	}
	if !strings.Contains(applied[0].Finding.Message, "FP-Memory Demoted") {
		t.Errorf("expected FP-Memory note in message, got: %s", applied[0].Finding.Message)
	}

	if applied[1].Finding.Severity != engines.High {
		t.Errorf("clean-rule should remain High, got: %s", applied[1].Finding.Severity)
	}
}
