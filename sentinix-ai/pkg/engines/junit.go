package engines

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type jUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type jUnitTestCase struct {
	Name      string         `xml:"name,attr"`
	ClassName string         `xml:"classname,attr"`
	Time      string         `xml:"time,attr"`
	Failures  []jUnitFailure `xml:"failure"`
	Errors    []jUnitFailure `xml:"error"`
}

type jUnitTestSuite struct {
	Name      string          `xml:"name,attr"`
	TestCases []jUnitTestCase `xml:"testcase"`
}

type jUnitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []jUnitTestSuite `xml:"testsuite"`
}

var (
	reStackLine  = regexp.MustCompile(`(?m)(?:at\s+[\w$.]+\s*\()?\s*([a-zA-Z0-9_./\\-]+(?:\.[a-zA-Z0-9]+)?):(\d+)\)?`)
	MaxJUnitSize = 10 << 20 // 10MB
)

// ErrOversized is returned when an input exceeds the maximum permitted size.
var ErrOversized = errors.New("input oversized")

// ErrPathTraversal is returned when an input contains a path traversal attempt.
var ErrPathTraversal = errors.New("path traversal detected")

// ParseJUnit parses a JUnit XML report (e.g. from Konsist or Gradle test runners)
// and maps each failing test to one Finding.
func ParseJUnit(engine string, data []byte, defaultCategory string) ([]Finding, error) {
	if len(data) > MaxJUnitSize {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrOversized, len(data), MaxJUnitSize)
	}

	var suites []jUnitTestSuite

	// Try unmarshaling as <testsuites>
	var rootSuites jUnitTestSuites
	if err := xml.Unmarshal(data, &rootSuites); err == nil && len(rootSuites.Suites) > 0 {
		suites = rootSuites.Suites
	} else {
		// Try unmarshaling as single <testsuite>
		var singleSuite jUnitTestSuite
		if err := xml.Unmarshal(data, &singleSuite); err == nil && (len(singleSuite.TestCases) > 0 || singleSuite.Name != "") {
			suites = []jUnitTestSuite{singleSuite}
		} else {
			return nil, fmt.Errorf("%s: bad JUnit XML: %w", engine, err)
		}
	}

	isKonsist := strings.Contains(strings.ToLower(engine), "konsist")
	cat := defaultCategory
	var sev Severity
	if isKonsist {
		if cat == "" {
			cat = CategoryArchitecture
		}
		sev = High
	} else {
		if cat == "" {
			cat = CategoryQuality
		}
		sev = Medium
	}

	var fs []Finding
	for _, suite := range suites {
		for _, tc := range suite.TestCases {
			fails := append([]jUnitFailure{}, tc.Failures...)
			fails = append(fails, tc.Errors...)

			for _, fail := range fails {
				rule := tc.ClassName
				if rule != "" && tc.Name != "" {
					rule = rule + "." + tc.Name
				} else if tc.Name != "" {
					rule = tc.Name
				} else if suite.Name != "" {
					rule = suite.Name
				}
				if rule == "" {
					rule = "unknown-test"
				}

				msg := fail.Message
				if msg == "" {
					lines := strings.Split(strings.TrimSpace(fail.Body), "\n")
					if len(lines) > 0 {
						msg = lines[0]
					}
				}
				if len(msg) > 500 {
					msg = msg[:500]
				}
				msg = Redact(msg)

				ev := strings.TrimSpace(fail.Body)
				if ev == "" {
					ev = fail.Message
				}
				if len(ev) > 200 {
					ev = ev[:200]
				}
				ev = Redact(ev)

				// Extract file and line from stack trace. Search body first, then message.
				var file string
				var line int

				targetText := fail.Body
				matches := reStackLine.FindStringSubmatch(targetText)
				if len(matches) != 3 {
					targetText = fail.Message
					matches = reStackLine.FindStringSubmatch(targetText)
				}

				if len(matches) == 3 {
					candidateFile := matches[1]
					cleanFile := filepath.ToSlash(filepath.Clean(candidateFile))
					// Path validation against traversal: skip hostile lines per finding (drop file/line)
					if strings.Contains(candidateFile, "..") || strings.HasPrefix(cleanFile, "../") || cleanFile == ".." || (filepath.IsAbs(candidateFile) && strings.Contains(candidateFile, "..")) {
						file = ""
						line = 0
					} else {
						cleanFile = strings.TrimPrefix(cleanFile, "/")
						file = cleanFile
						if l, err := strconv.Atoi(matches[2]); err == nil {
							line = l
						}
					}
				}

				f := Finding{
					Rule:     rule,
					Engine:   engine,
					Category: cat,
					File:     file,
					Line:     line,
					Severity: sev,
					Message:  msg,
					Evidence: ev,
					Source:   "ingest",
				}

				if ev != "" {
					h := sha256.Sum256([]byte(ev))
					f.Match = hex.EncodeToString(h[:])
				}

				fs = append(fs, f)
			}
		}
	}

	return fs, nil
}
