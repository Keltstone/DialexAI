package hunt

import (
	"context"
	"strings"
	"testing"

	"sentinix/pkg/engines"
)

func TestParseCodeQLSARIFWithCodeFlows(t *testing.T) {
	sarifJSON := `{
  "version": "2.1.0",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "CodeQL"
        }
      },
      "results": [
        {
          "ruleId": "java/sql-injection",
          "level": "error",
          "message": {
            "text": "Query built from untrusted input"
          },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": {
                  "uri": "app/src/DbHelper.java"
                },
                "region": {
                  "startLine": 45
                }
              }
            }
          ],
          "codeFlows": [
            {
              "threadFlows": [
                {
                  "id": "thread-1",
                  "locations": [
                    {
                      "location": {
                        "message": { "text": "getParameter" },
                        "physicalLocation": {
                          "artifactLocation": { "uri": "app/src/ApiController.java" },
                          "region": { "startLine": 12, "snippet": { "text": "String id = req.getParameter(\"id\");" } }
                        }
                      }
                    },
                    {
                      "location": {
                        "message": { "text": "pass to helper" },
                        "physicalLocation": {
                          "artifactLocation": { "uri": "app/src/Service.java" },
                          "region": { "startLine": 28, "snippet": { "text": "db.findUser(id);" } }
                        }
                      }
                    },
                    {
                      "location": {
                        "message": { "text": "raw query execution" },
                        "physicalLocation": {
                          "artifactLocation": { "uri": "app/src/DbHelper.java" },
                          "region": { "startLine": 45, "snippet": { "text": "stmt.execute(\"SELECT * FROM users WHERE id=\" + id);" } }
                        }
                      }
                    }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}`

	findings, err := ParseCodeQLSARIF([]byte(sarifJSON))
	if err != nil {
		t.Fatalf("ParseCodeQLSARIF failed: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	f := findings[0]
	if f.Rule != "java/sql-injection" || f.File != "app/src/DbHelper.java" || f.Line != 45 {
		t.Errorf("unexpected finding metadata: %+v", f)
	}

	if len(f.Flows) != 1 || len(f.Flows[0].Steps) != 3 {
		t.Fatalf("expected 1 flow with 3 steps, got %+v", f.Flows)
	}

	source := f.Flows[0].Steps[0]
	sink := f.Flows[0].Steps[2]

	if source.File != "app/src/ApiController.java" || source.Line != 12 {
		t.Errorf("unexpected source: %+v", source)
	}
	if sink.File != "app/src/DbHelper.java" || sink.Line != 45 {
		t.Errorf("unexpected sink: %+v", sink)
	}

	trace := FormatFlowTrace(f.Flows[0])
	if !strings.Contains(trace, "[source] app/src/ApiController.java:12") ||
		!strings.Contains(trace, "[sink] app/src/DbHelper.java:45") {
		t.Errorf("unexpected trace: %s", trace)
	}
}

func TestCodeQLCacheKey(t *testing.T) {
	k1 := CodeQLCacheKey("abc123456789", "java")
	k2 := CodeQLCacheKey("abc123456789", "JAVA")
	k3 := CodeQLCacheKey("diffcommit99", "java")

	if k1 != k2 {
		t.Errorf("expected case-insensitive language cache key: %s vs %s", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("expected different keys for different commits: %s vs %s", k1, k3)
	}
	if !strings.HasPrefix(k1, "codeql-java-") {
		t.Errorf("unexpected key format: %s", k1)
	}
}

type semgrepGenFake struct{}

func (f semgrepGenFake) Name() string { return "semgrep-gen" }
func (f semgrepGenFake) Local() bool  { return true }
func (f semgrepGenFake) Complete(_ context.Context, _, _ string) (string, error) {
	return `{"id":"sentinix-raw-sql","pattern":"stmt\\.execute\\(.*\\+.*\\)","message":"potential sql injection","severity":"ERROR","languages":["java"]}`, nil
}

func TestSemgrepRuleGenerationAndVerification(t *testing.T) {
	fakeModel := semgrepGenFake{}
	finding := engines.Finding{
		Rule:    "hunt/sql-injection",
		File:    "DbHelper.java",
		Message: "Raw query execution with user parameter",
	}

	code := `void query(String id) { stmt.execute("SELECT * FROM x WHERE id=" + id); }`

	rule, err := GenerateSemgrepRule(context.Background(), fakeModel, finding, code)
	if err != nil {
		t.Fatalf("GenerateSemgrepRule failed: %v", err)
	}

	if rule.ID != "sentinix-raw-sql" || rule.Severity != "ERROR" {
		t.Errorf("unexpected rule metadata: %+v", rule)
	}

	// Verify positive fixture match
	vulnerableCode := `stmt.execute("SELECT * FROM users WHERE name=" + userInput);`
	if err := VerifyRuleFixture(rule, vulnerableCode, true); err != nil {
		t.Errorf("expected vulnerable code to match rule: %v", err)
	}

	// Verify negative fixture match (clean parameterized code)
	cleanCode := `stmt.execute("SELECT * FROM users WHERE name=?", userInput);`
	if err := VerifyRuleFixture(rule, cleanCode, false); err != nil {
		t.Errorf("expected clean code not to match rule: %v", err)
	}

	// Quarantine wrapping
	quarantined := QuarantineRule(rule, "user@sentinix.local")
	if quarantined.Status != "quarantined" || quarantined.Proposer != "user@sentinix.local" {
		t.Errorf("unexpected quarantine state: %+v", quarantined)
	}
}
