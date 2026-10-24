package triage

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"time"
)

// JUnitTestCase models a single test case execution for CI consumption.
type JUnitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
	Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
}

// JUnitFailure records a test assertion or execution failure.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitSkipped records a skipped or quarantined test.
type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

// JUnitTestSuite models a collection of test cases.
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	Timestamp string          `xml:"timestamp,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// TestCaseResult represents input result for JUnit export.
type TestCaseResult struct {
	Name      string
	SuiteName string
	Duration  time.Duration
	Passed    bool
	Skipped   bool
	ErrorMsg  string
	ErrorType string
	Details   string
}

// ExportJUnitXML serializes test results to standard JUnit XML (compatible with GitHub Actions, Jenkins, CircleCI).
func ExportJUnitXML(suiteName string, results []TestCaseResult) ([]byte, error) {
	suite := JUnitTestSuite{
		Name:      suiteName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tests:     len(results),
	}

	var totalTime float64
	for _, r := range results {
		durSec := r.Duration.Seconds()
		totalTime += durSec

		tc := JUnitTestCase{
			Name:      r.Name,
			Classname: r.SuiteName,
			Time:      durSec,
		}

		if r.Skipped {
			suite.Skipped++
			tc.Skipped = &JUnitSkipped{Message: r.ErrorMsg}
		} else if !r.Passed {
			suite.Failures++
			tc.Failure = &JUnitFailure{
				Message: r.ErrorMsg,
				Type:    r.ErrorType,
				Content: r.Details,
			}
		}

		suite.TestCases = append(suite.TestCases, tc)
	}

	suite.Time = totalTime
	output, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JUnit XML: %w", err)
	}

	return append([]byte(xml.Header), output...), nil
}

// SARIFReport models OASIS SARIF 2.1.0 output for enterprise vulnerability dashboards.
type SARIFReport struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

type SARIFDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []SARIFRule `json:"rules"`
}

type SARIFRule struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	ShortDescription SARIFDescription `json:"shortDescription"`
}

type SARIFDescription struct {
	Text string `json:"text"`
}

type SARIFResult struct {
	RuleID    string           `json:"ruleId"`
	Level     string           `json:"level"` // "error", "warning", "note"
	Message   SARIFDescription `json:"message"`
	Locations []SARIFLocation  `json:"locations,omitempty"`
}

type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
}

type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

// SecurityFindingInput represents an identified vulnerability from fuzzing or scans.
type SecurityFindingInput struct {
	RuleID      string
	Title       string
	Severity    string // "blocker", "critical", "major", "minor"
	Description string
	TargetURL   string
}

// ExportSARIF converts security/fuzzing findings into OASIS SARIF 2.1.0 format.
func ExportSARIF(findings []SecurityFindingInput) ([]byte, error) {
	var rules []SARIFRule
	var results []SARIFResult
	ruleSet := make(map[string]bool)

	for _, f := range findings {
		if !ruleSet[f.RuleID] {
			rules = append(rules, SARIFRule{
				ID:               f.RuleID,
				Name:             f.Title,
				ShortDescription: SARIFDescription{Text: f.Description},
			})
			ruleSet[f.RuleID] = true
		}

		level := "warning"
		if f.Severity == "blocker" || f.Severity == "critical" {
			level = "error"
		} else if f.Severity == "minor" {
			level = "note"
		}

		results = append(results, SARIFResult{
			RuleID:  f.RuleID,
			Level:   level,
			Message: SARIFDescription{Text: f.Description},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{URI: f.TargetURL},
					},
				},
			},
		})
	}

	report := SARIFReport{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []SARIFRun{
			{
				Tool: SARIFTool{
					Driver: SARIFDriver{
						Name:           "Kritix-DAST",
						Version:        "1.0.0",
						InformationURI: "https://github.com/Keltstone/DialexAI/tree/main/kritix-ai",
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	return json.MarshalIndent(report, "", "  ")
}
