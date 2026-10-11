package reviewer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var defaultProtectedPrefixes = []string{
	".github/",
	".gitlab-ci",
	".circleci/",
	".artix/",
	".kritix/",
	"policy.json",
	"steering.json",
	"AGENTS.md",
	"CLAUDE.md",
	"GEMINI.md",
	"Makefile",
	"package.json",
	"package-lock.json",
	"build.gradle",
	"build.gradle.kts",
	"pom.xml",
	"go.mod",
	"go.sum",
	"Cargo.toml",
	"requirements.txt",
	"Pipfile",
	"pyproject.toml",
}

// CheckProtectedPaths verifies that unified diffs do not touch CI, steering, or governance files.
func CheckProtectedPaths(diff string, customProtected ...string) []string {
	var violations []string
	protected := append(defaultProtectedPrefixes, customProtected...)

	files := extractTouchedFiles(diff)
	for _, f := range files {
		clean := filepath.Clean(f)
		clean = strings.TrimPrefix(clean, "./")
		base := filepath.Base(clean)

		isProtected := false
		// 1. Any path containing .github/, .gitlab/, or .circleci/ anywhere
		if strings.Contains(clean, ".github/") || strings.HasSuffix(clean, ".github") || strings.Contains(clean, "/.github/") ||
			strings.Contains(clean, ".gitlab-ci") || strings.Contains(clean, ".circleci/") {
			isProtected = true
		}
		// 2. CODEOWNERS anywhere
		if base == "CODEOWNERS" || strings.Contains(clean, "CODEOWNERS") {
			isProtected = true
		}
		// 3. Git metadata and hooks anywhere
		if strings.HasPrefix(clean, ".git/") || clean == ".git" || strings.Contains(clean, "/.git/") ||
			strings.Contains(clean, ".githooks") || strings.Contains(clean, "githooks/") || strings.Contains(clean, ".git/hooks") ||
			base == "pre-commit" || base == "post-commit" || base == "pre-push" || base == "commit-msg" {
			isProtected = true
		}
		// 4. Default protected prefixes and custom protected
		if !isProtected {
			for _, p := range protected {
				if strings.HasPrefix(clean, p) || clean == p || base == p {
					isProtected = true
					break
				}
			}
		}

		if isProtected {
			violations = append(violations, fmt.Sprintf("protected path violation: modification to protected system/governance file %q is strictly blocked", f))
		}
	}
	return violations
}

var reStringLit = regexp.MustCompile(`"(\\.|[^"\\])*"|` + "`[^`]*`")

func stripStringLiterals(s string) string {
	return reStringLit.ReplaceAllString(s, `""`)
}

var reTautology1 = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_&|^+\-*/%])1\s*==\s*1\b|(?:^|[^a-zA-Z0-9_&|^+\-*/%])1\s*!=\s*1\b|\bassert\.True\(t,\s*true\)|\bassert\.False\(t,\s*false\)|\bassert\.Equal\(t,\s*1,\s*1\)|\bif\s+!true\b`)
var reImpossibleLen = regexp.MustCompile(`(?:len|size)\([^)]*\)\s*(?:<\s*0|<=\s*-1|==\s*-1|>\s*1\s*<<\s*6[0-9])`)

func matchedTautology(s string) bool {
	return reTautology1.MatchString(s)
}

// CheckTestIntegrity ensures tests are not deleted, assertions gutted, or test skipping introduced.
func CheckTestIntegrity(diff string) []string {
	var violations []string
	lines := strings.Split(diff, "\n")

	var deletedAssertions int
	var addedAssertions int

	for i, line := range lines {
		// Ignore diff header lines
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}

		// Detect test deletions
		if strings.HasPrefix(line, "-") {
			trimmed := strings.TrimSpace(line[1:])
			if strings.HasPrefix(trimmed, "func Test") {
				violations = append(violations, fmt.Sprintf("test integrity violation: test function deletion detected: %s", trimmed))
			} else if strings.HasPrefix(trimmed, "@Test") || strings.HasPrefix(trimmed, "fun test") {
				violations = append(violations, fmt.Sprintf("test integrity violation: test method deletion detected: %s", trimmed))
			} else if strings.HasPrefix(trimmed, "it(") || strings.HasPrefix(trimmed, "test(") || strings.HasPrefix(trimmed, "describe(") {
				violations = append(violations, fmt.Sprintf("test integrity violation: test block deletion detected: %s", trimmed))
			} else if strings.HasPrefix(trimmed, "def test_") {
				violations = append(violations, fmt.Sprintf("test integrity violation: Python test deletion detected: %s", trimmed))
			}

			// Track removed assertions
			if isAssertionStatement(trimmed) {
				deletedAssertions++
			}
		}

		// Detect test skips & gutted bodies
		if strings.HasPrefix(line, "+") {
			trimmed := strings.TrimSpace(line[1:])
			// Exclude comments
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "#") {
				continue
			}

			if (strings.Contains(trimmed, "t.Skip(") || strings.Contains(trimmed, "t.SkipNow()") || strings.Contains(trimmed, "t.Skip") || strings.Contains(trimmed, "t.Skipf")) &&
				!strings.Contains(diff, "testing.Short()") && !strings.Contains(trimmed, "Short()") {
				violations = append(violations, "test integrity violation: Go test skipping is forbidden (t.Skip)")
			} else if strings.Contains(trimmed, "@Ignore") || strings.Contains(trimmed, "@Disabled") {
				violations = append(violations, "test integrity violation: test disablement is forbidden (@Ignore/@Disabled)")
			} else if strings.Contains(trimmed, ".skip(") || strings.Contains(trimmed, "xit(") || strings.Contains(trimmed, "xtest(") || strings.Contains(trimmed, "xdescribe(") || strings.Contains(trimmed, "it.skip(") || strings.Contains(trimmed, "test.skip(") {
				violations = append(violations, "test integrity violation: test skipping is forbidden (.skip/xit/xtest/xdescribe)")
			} else if strings.Contains(trimmed, "@pytest.mark.skip") || strings.Contains(trimmed, "@unittest.skip") {
				violations = append(violations, "test integrity violation: test skipping is forbidden (@skip)")
			}

			// Detect gutted test function bodies e.g. func TestFoo(t *testing.T) {} or func TestFoo(t *testing.T) { return }
			if strings.HasPrefix(trimmed, "func Test") && (strings.HasSuffix(trimmed, "{}") || strings.HasSuffix(trimmed, "{ return }") || strings.HasSuffix(trimmed, "{ return; }")) {
				violations = append(violations, fmt.Sprintf("test integrity violation: gutted empty test body detected: %s", trimmed))
			} else if strings.HasPrefix(trimmed, "func Test") && i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if nextLine == "+}" || nextLine == "+  return" || nextLine == "+	return" {
					violations = append(violations, fmt.Sprintf("test integrity violation: gutted test function with early return: %s", trimmed))
				}
			}

			codeWithoutStrings := stripStringLiterals(trimmed)

			// Detect impossible/weakened assertion
			if reImpossibleLen.MatchString(codeWithoutStrings) {
				violations = append(violations, fmt.Sprintf("test integrity violation: impossible/weakened assertion detected (%s)", trimmed))
			}

			// Detect tautology / dead test condition (e.g. 1 != 1, 1 == 1, if false, assert.True(t, true))
			if matchedTautology(codeWithoutStrings) {
				violations = append(violations, fmt.Sprintf("test integrity violation: tautological assertion / dead test condition detected (%s)", trimmed))
			}

			// Detect assertion inside loop over empty slice/collection e.g. for range []string{} { t.Fatal(...) }
			isEmptyRangeLit := false
			if strings.Contains(codeWithoutStrings, "range ") {
				if matched, _ := regexp.MatchString(`range\s+(\[\]|[a-zA-Z0-9_\.]*map\[)[a-zA-Z0-9_\.\]\s]*\{\s*\}`, codeWithoutStrings); matched {
					isEmptyRangeLit = true
				}
			}
			if isEmptyRangeLit || strings.Contains(codeWithoutStrings, "for i := 0; i < 0;") || strings.Contains(codeWithoutStrings, "for i := 0; i <= -1;") {
				violations = append(violations, fmt.Sprintf("test integrity violation: assertion inside empty loop detected (%s)", trimmed))
			}

			if isAssertionStatement(trimmed) {
				addedAssertions++
			}
		}
	}

	// Flag any net assertion loss
	if deletedAssertions > addedAssertions {
		violations = append(violations, fmt.Sprintf("test integrity violation: net assertion loss detected (%d assertion statements removed, %d added)", deletedAssertions, addedAssertions))
	}

	// AST-based assertion reachability analysis on added Go test code
	addedLines := extractAddedCodeLines(diff)
	if len(addedLines) > 0 {
		codeBlob := strings.Join(addedLines, "\n")
		if strings.Contains(codeBlob, "func ") || strings.Contains(codeBlob, "t *testing.T") || strings.Contains(codeBlob, "t.") || strings.Contains(codeBlob, "testing.") || strings.Contains(codeBlob, "assert.") || strings.Contains(codeBlob, "defer ") || strings.Contains(codeBlob, "import ") {
			fset := token.NewFileSet()
			var parsedNodes []*ast.File
			balancedCode := codeBlob
			openCount := strings.Count(balancedCode, "{")
			closeCount := strings.Count(balancedCode, "}")
			if openCount > closeCount {
				balancedCode += strings.Repeat("\n}", openCount-closeCount)
			}
			if strings.Contains(balancedCode, "package ") {
				if node, err := parser.ParseFile(fset, "diff_direct.go", balancedCode, parser.ParseComments); err == nil {
					parsedNodes = append(parsedNodes, node)
				}
			} else if strings.Contains(balancedCode, "func ") {
				if node, err := parser.ParseFile(fset, "diff_pkg.go", fmt.Sprintf("package p\n%s\n", balancedCode), parser.ParseComments); err == nil {
					parsedNodes = append(parsedNodes, node)
				}
			} else {
				if node, err := parser.ParseFile(fset, "diff_func.go", fmt.Sprintf("package p\nfunc _(t *testing.T) {\n%s\n}\n", balancedCode), parser.ParseComments); err == nil {
					parsedNodes = append(parsedNodes, node)
				}
			}
			for _, node := range parsedNodes {
				violations = append(violations, inspectTestASTIntegrity(node)...)
			}
		}
	}

	return deduplicateStrings(violations)
}

func isAssertionStatement(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "assert.") ||
		strings.Contains(lower, "require.") ||
		strings.Contains(lower, "t.fatal") ||
		strings.Contains(lower, "t.error") ||
		strings.Contains(lower, "assertequals") ||
		strings.Contains(lower, "asserttrue") ||
		strings.Contains(lower, "assertfalse") ||
		strings.Contains(lower, "expect(")
}

func formatExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		return e.Value
	case *ast.SelectorExpr:
		return formatExpr(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		var args []string
		for _, a := range e.Args {
			args = append(args, formatExpr(a))
		}
		return formatExpr(e.Fun) + "(" + strings.Join(args, ", ") + ")"
	case *ast.ParenExpr:
		return formatExpr(e.X)
	case *ast.UnaryExpr:
		return e.Op.String() + formatExpr(e.X)
	case *ast.BinaryExpr:
		return formatExpr(e.X) + " " + e.Op.String() + " " + formatExpr(e.Y)
	default:
		return fmt.Sprintf("%v", e)
	}
}

func isAssertionCall(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if ident, ok := call.Fun.(*ast.Ident); ok {
		name := strings.ToLower(ident.Name)
		if strings.HasPrefix(name, "assert") || strings.HasPrefix(name, "check") || strings.HasPrefix(name, "verify") || strings.HasPrefix(name, "chk") || strings.HasPrefix(name, "equal") || strings.HasPrefix(name, "expect") {
			return true
		}
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		name := strings.ToLower(sel.Sel.Name)
		if name == "fatal" || name == "fatalf" || name == "error" || name == "errorf" || name == "fail" || name == "failnow" ||
			strings.HasPrefix(name, "assert") || strings.HasPrefix(name, "require") ||
			strings.HasPrefix(name, "check") || strings.HasPrefix(name, "verify") ||
			strings.HasPrefix(name, "equal") || strings.HasPrefix(name, "is") || strings.HasPrefix(name, "match") ||
			strings.HasPrefix(name, "expect") {
			return true
		}
	}
	return false
}

// CheckSemanticASTTaboos checks diff against taboo patterns using AST parsing for Go and comment-aware filtering.
func CheckSemanticASTTaboos(diff string, tabooList []string, workspaceDir ...string) []string {
	var violations []string
	if strings.TrimSpace(diff) == "" {
		return violations
	}

	// 1. If workspaceDir is provided, scan full post-patch tree (all source files)
	var wholeFiles []*ast.File
	fset := token.NewFileSet()
	if len(workspaceDir) > 0 && workspaceDir[0] != "" {
		root := workspaceDir[0]
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			name := info.Name()
			if info.IsDir() {
				if name == ".git" || name == "vendor" || name == "node_modules" || name == ".artix" || (strings.HasPrefix(name, ".") && name != ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(name, ".go") {
				if node, err := parser.ParseFile(fset, path, nil, parser.ParseComments); err == nil {
					wholeFiles = append(wholeFiles, node)
				}
			} else if strings.HasSuffix(name, ".py") || strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".kt") || strings.HasSuffix(name, ".swift") {
				if content, err := os.ReadFile(path); err == nil {
					violations = append(violations, inspectMultiLanguageSemantics(string(content), tabooList)...)
				}
			}
			return nil
		})
	}

	// 2. Extract added code lines (excluding comments)
	addedCodeLines := extractAddedCodeLines(diff)
	addedCodeBlob := strings.Join(addedCodeLines, "\n")

	// 2b. Inherent script attack detection: curl ... | sh or wget ... | sh / bash
	for _, l := range addedCodeLines {
		lLower := strings.ToLower(l)
		if (strings.Contains(lLower, "curl") || strings.Contains(lLower, "wget")) && (strings.Contains(lLower, "| sh") || strings.Contains(lLower, "| bash") || strings.Contains(lLower, "|sh") || strings.Contains(lLower, "|bash") || strings.Contains(lLower, "| zsh")) {
			violations = append(violations, fmt.Sprintf("Security Taboo: forbidden remote script pipe execution (%s)", l))
		}
	}

	// 3. Perform Go AST analysis on Go code snippets and whole files if present
	astViolations := inspectGoASTForTaboos(addedCodeBlob, tabooList, wholeFiles...)
	violations = append(violations, astViolations...)

	// 4. Multi-language semantic rule matching for Kotlin, Swift, TypeScript, Python
	semanticViolations := inspectMultiLanguageSemantics(addedCodeBlob, tabooList)
	violations = append(violations, semanticViolations...)

	return deduplicateStrings(violations)
}

func extractTouchedFiles(diff string) []string {
	var files []string
	lines := strings.Split(diff, "\n")
	reDiff := regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
	rePlus := regexp.MustCompile(`^\+\+\+ b/(.*)$`)

	for _, line := range lines {
		if m := reDiff.FindStringSubmatch(line); len(m) > 2 {
			files = append(files, m[2])
		} else if m := rePlus.FindStringSubmatch(line); len(m) > 1 {
			if m[1] != "/dev/null" {
				files = append(files, m[1])
			}
		}
	}
	return deduplicateStrings(files)
}

func extractAddedCodeLines(diff string) []string {
	var lines []string
	for _, l := range strings.Split(diff, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			code := strings.TrimSpace(l[1:])
			// Filter out pure comment lines to prevent false-positives (except directives like go:linkname)
			if strings.HasPrefix(code, "//go:") || strings.HasPrefix(code, "+build") {
				lines = append(lines, code)
				continue
			}
			if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "/*") || strings.HasPrefix(code, "*") || strings.HasPrefix(code, "#") {
				continue
			}
			if code != "" {
				lines = append(lines, code)
			}
		}
	}
	return lines
}

func transitivelyHasProp(fnName string, calls map[string][]string, prop map[string]bool, visited map[string]bool) bool {
	if prop[fnName] {
		return true
	}
	if visited[fnName] {
		return false
	}
	visited[fnName] = true
	for _, callee := range calls[fnName] {
		if transitivelyHasProp(callee, calls, prop, visited) {
			return true
		}
	}
	return false
}

func isSensitiveEnvKey(key string) bool {
	kUpper := strings.ToUpper(strings.Trim(key, "\"'`"))
	nonSecrets := map[string]bool{
		"PORT": true, "HOST": true, "ENV": true, "NODE_ENV": true, "APP_ENV": true,
		"STAGE": true, "DEBUG": true, "LOG_LEVEL": true, "CI": true, "LANG": true,
		"TZ": true, "TERM": true, "USER": true, "HOME": true, "PATH": true, "SHELL": true,
		"GOOS": true, "GOARCH": true, "GOROOT": true, "GOPATH": true,
	}
	if nonSecrets[kUpper] {
		return false
	}
	if strings.Contains(kUpper, "TOKEN") || strings.Contains(kUpper, "KEY") ||
		strings.Contains(kUpper, "SECRET") || strings.Contains(kUpper, "PASSWORD") ||
		strings.Contains(kUpper, "PASS") || strings.Contains(kUpper, "AUTH") ||
		strings.Contains(kUpper, "CRED") || strings.Contains(kUpper, "PRIVATE") ||
		strings.Contains(kUpper, "DATABASE_URL") || strings.Contains(kUpper, "APIKEY") ||
		strings.Contains(kUpper, "ACCESS") || strings.Contains(kUpper, "SIGNING") ||
		strings.Contains(kUpper, "CERT") || strings.Contains(kUpper, "SALT") {
		return true
	}
	return false
}

func inspectGoASTForTaboos(code string, taboos []string, wholeFiles ...*ast.File) []string {
	var violations []string
	if code == "" && len(wholeFiles) == 0 {
		return violations
	}

	isTestCode := strings.Contains(code, "func Test") || strings.Contains(code, "t *testing.T") || strings.Contains(code, "testing.") || strings.Contains(code, "_test.go") || strings.Contains(code, "func Benchmark") || strings.Contains(code, "func Example") || strings.Contains(code, "func Fuzz")

	// Directive checks: //go:linkname in non-test code
	if !isTestCode && strings.Contains(code, "go:linkname") {
		violations = append(violations, "AST Taboo Violation: forbidden //go:linkname directive in non-test code")
	}

	taboosJoined := strings.ToLower(strings.Join(taboos, " "))
	checkDefaultClient := strings.Contains(taboosJoined, "defaultclient")
	checkDefaultServeMux := strings.Contains(taboosJoined, "defaultservemux")
	checkDefaultTransport := strings.Contains(taboosJoined, "defaulttransport")
	checkInsecureTLS := strings.Contains(taboosJoined, "insecureskipverify") || strings.Contains(taboosJoined, "tls")
	checkShellExec := strings.Contains(taboosJoined, "exec.command") || strings.Contains(taboosJoined, "shell") || strings.Contains(taboosJoined, "raw shell") || true // Inherent taboo
	checkReflection := strings.Contains(taboosJoined, "reflection") || strings.Contains(taboosJoined, "reflect")

	// Multi-strategy parsing: handles imports, package-level declarations, function bodies, and bare statements
	var parsedFiles []*ast.File
	parsedFiles = append(parsedFiles, wholeFiles...)
	fset := token.NewFileSet()

	// Strategy 0: Direct parse if code already contains a package declaration
	if strings.Contains(code, "package ") {
		if node, err := parser.ParseFile(fset, "diff_direct.go", code, parser.ParseComments); err == nil {
			parsedFiles = append(parsedFiles, node)
		}
	}

	// Strategy A: parse as package-level declarations (handles imports, types, consts, funcs)
	if node, err := parser.ParseFile(fset, "diff_pkg.go", fmt.Sprintf("package p\n%s\n", code), parser.ParseComments); err == nil {
		parsedFiles = append(parsedFiles, node)
	}

	// Strategy B: parse wrapped in function body (handles bare statements, expressions, local assignments)
	if node, err := parser.ParseFile(fset, "diff_func.go", fmt.Sprintf("package p\nfunc _() {\n%s\n}\n", code), parser.ParseComments); err == nil {
		parsedFiles = append(parsedFiles, node)
	}

	if len(parsedFiles) == 0 {
		return violations
	}

	for _, node := range parsedFiles {
		// Check comments for directives in non-test code
		if !isTestCode {
			for _, cg := range node.Comments {
				for _, c := range cg.List {
					if strings.Contains(c.Text, "go:linkname") {
						violations = append(violations, "AST Taboo Violation: forbidden //go:linkname directive in non-test code")
					}
				}
			}
		}

		// Track import aliases: map of local identifier -> package path
		importAliases := make(map[string]string)
		dotImports := make(map[string]bool)
		for _, imp := range node.Imports {
			pkgPath := strings.Trim(imp.Path.Value, `"`)
			alias := filepath.Base(pkgPath)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias == "." {
				dotImports[pkgPath] = true
			} else if alias != "_" {
				importAliases[alias] = pkgPath
			}

			// Check if import itself violates package taboo
			for _, taboo := range taboos {
				tLower := strings.ToLower(taboo)
				if strings.Contains(tLower, strings.ToLower(pkgPath)) || (pkgPath == "net/http" && strings.Contains(tLower, "net/http")) || (pkgPath == "os/exec" && strings.Contains(tLower, "os/exec")) || (pkgPath == "reflect" && (tLower == "reflect" || tLower == "reflection")) {
					violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden import of package %q (alias %q)", pkgPath, alias))
				}
			}

			// Check reflection import
			if checkReflection && (pkgPath == "reflect" || strings.Contains(pkgPath, "reflect")) {
				violations = append(violations, "AST Taboo Violation: forbidden import of reflect package (reflection taboo)")
			}
		}

		stringConsts := make(map[string]string)
		var resolveStringExpr func(expr ast.Expr) string
		resolveStringExpr = func(expr ast.Expr) string {
			if expr == nil {
				return ""
			}
			switch e := expr.(type) {
			case *ast.BasicLit:
				if e.Kind == token.STRING {
					return strings.Trim(e.Value, "`\"")
				}
			case *ast.BinaryExpr:
				if e.Op == token.ADD {
					return resolveStringExpr(e.X) + resolveStringExpr(e.Y)
				}
			case *ast.Ident:
				if val, ok := stringConsts[e.Name]; ok {
					return val
				}
				return e.Name
			case *ast.CallExpr:
				var parts []string
				for _, arg := range e.Args {
					parts = append(parts, resolveStringExpr(arg))
				}
				return strings.Join(parts, "")
			}
			return ""
		}

		// Taint tracking data structures
		taintedVars := make(map[string]bool)
		taintedFuncs := make(map[string]bool)
		taintedFields := make(map[string]bool)
		taintedReceivers := make(map[string]bool)
		aliasMap := make(map[string]string)
		ptrAliasMap := make(map[string]string)
		dsnExternalMap := make(map[string]bool)
		funcCalls := make(map[string][]string)
		funcHasNet := make(map[string]bool)
		funcHasSecret := make(map[string]bool)
		funcHasSink := make(map[string]bool)

		isExternalHostDSN := func(expr ast.Expr) bool {
			if id, ok := expr.(*ast.Ident); ok {
				return dsnExternalMap[id.Name]
			}
			var allStrings []string
			ast.Inspect(expr, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					val := strings.Trim(lit.Value, "`\"")
					allStrings = append(allStrings, val)
				}
				return true
			})
			if len(allStrings) == 0 {
				return true
			}
			combined := strings.ToLower(strings.Join(allStrings, " "))
			if strings.Contains(combined, "localhost") ||
				strings.Contains(combined, "127.0.0.1") ||
				strings.Contains(combined, "::1") ||
				strings.Contains(combined, ":memory:") ||
				strings.Contains(combined, ".db") ||
				strings.Contains(combined, ".sqlite") {
				return false
			}
			if strings.Contains(combined, "@") || strings.Contains(combined, "://") ||
				strings.Contains(combined, "tcp(") || strings.Contains(combined, "host=") {
				return true
			}
			return true
		}

		var isSecretExpr func(expr ast.Expr) bool
		isSecretExpr = func(expr ast.Expr) bool {
			if expr == nil {
				return false
			}
			switch e := expr.(type) {
			case *ast.Ident:
				nameUpper := strings.ToUpper(e.Name)
				if taintedVars[e.Name] {
					return true
				}
				if strings.Contains(nameUpper, "SECRET") || strings.Contains(nameUpper, "PASSWORD") ||
					strings.Contains(nameUpper, "API_KEY") || strings.Contains(nameUpper, "AUTH_TOKEN") ||
					strings.Contains(nameUpper, "ACCESS_KEY") || strings.Contains(nameUpper, "DB_PASSWORD") {
					return true
				}
			case *ast.CallExpr:
				funStr := formatExpr(e.Fun)
				funLower := strings.ToLower(funStr)
				if funLower == "os.getenv" || funLower == "os.lookupenv" || funLower == "getenv" || funLower == "lookupenv" {
					if len(e.Args) > 0 {
						if lit, ok := e.Args[0].(*ast.BasicLit); ok {
							if isSensitiveEnvKey(lit.Value) {
								return true
							}
							return false // Literal is non-sensitive e.g. "PORT"
						}
						// Dynamic or variable key expression treated as secret source
						return true
					}
					return true
				}
				if funLower == "os.environ" || funLower == "environ" {
					return true // All environment variables
				}
				if ident, ok := e.Fun.(*ast.Ident); ok && taintedFuncs[ident.Name] {
					return true
				}
				if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
					if taintedFuncs[sel.Sel.Name] {
						return true
					}
					if isSecretExpr(sel.X) {
						return true
					}
					// Method chains where receiver or any arg is tainted
					for _, arg := range e.Args {
						if isSecretExpr(arg) {
							return true
						}
					}
				}
				if strings.HasSuffix(funLower, ".get") || strings.HasSuffix(funLower, ".getstring") || strings.HasSuffix(funLower, "get") {
					for _, arg := range e.Args {
						if argLit, ok := arg.(*ast.BasicLit); ok {
							valUpper := strings.ToUpper(argLit.Value)
							if strings.Contains(valUpper, "SECRET") || strings.Contains(valUpper, "KEY") ||
								strings.Contains(valUpper, "TOKEN") || strings.Contains(valUpper, "PASSWORD") ||
								strings.Contains(valUpper, "AUTH") || strings.Contains(valUpper, "ACCESS") {
								return true
							}
						}
					}
				}
				if funLower == "os.readfile" || funLower == "os.open" || funLower == "readfile" {
					for _, arg := range e.Args {
						argStr := strings.ToLower(formatExpr(arg))
						if strings.Contains(argStr, "credential") || strings.Contains(argStr, ".aws") ||
							strings.Contains(argStr, ".ssh") || strings.Contains(argStr, "secret") {
							return true
						}
					}
				}
				for _, arg := range e.Args {
					if isSecretExpr(arg) {
						return true
					}
				}
			case *ast.SelectorExpr:
				selUpper := strings.ToUpper(e.Sel.Name)
				if strings.Contains(selUpper, "KEY") || strings.Contains(selUpper, "SECRET") ||
					strings.Contains(selUpper, "TOKEN") || strings.Contains(selUpper, "PASSWORD") {
					return true
				}
				exprStr := formatExpr(e)
				if taintedVars[exprStr] || taintedFields[e.Sel.Name] {
					return true
				}
				if isSecretExpr(e.X) {
					return true
				}
				if id, ok := e.X.(*ast.Ident); ok && taintedVars[id.Name] {
					return true
				}
			case *ast.IndexExpr:
				exprStr := formatExpr(e)
				if taintedVars[exprStr] {
					return true
				}
				if isSecretExpr(e.X) {
					return true
				}
				if id, ok := e.X.(*ast.Ident); ok && taintedVars[id.Name] {
					return true
				}
			case *ast.CompositeLit:
				for _, elt := range e.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if isSecretExpr(kv.Value) {
							return true
						}
					} else if isSecretExpr(elt) {
						return true
					}
				}
			case *ast.UnaryExpr:
				return isSecretExpr(e.X)
			case *ast.BinaryExpr:
				return isSecretExpr(e.X) || isSecretExpr(e.Y)
			case *ast.ParenExpr:
				return isSecretExpr(e.X)
			}
			return false
		}

		// Pass 1: Scan for tainted variables, tainted struct fields, and functions returning tainted expressions
		for pass := 0; pass < 5; pass++ {
			for _, decl := range node.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
					fnName := fn.Name.Name
					ast.Inspect(fn.Body, func(in ast.Node) bool {
						if assign, ok := in.(*ast.AssignStmt); ok {
							for i, rhs := range assign.Rhs {
								rhsTainted := isSecretExpr(rhs)
								if rhsTainted && i < len(assign.Lhs) {
									lhs := assign.Lhs[i]
									if lhsIdent, ok := lhs.(*ast.Ident); ok {
										taintedVars[lhsIdent.Name] = true
										funcHasSecret[fnName] = true
										if isExternalHostDSN(rhs) {
											dsnExternalMap[lhsIdent.Name] = true
										}
									}
									if sel, ok := lhs.(*ast.SelectorExpr); ok {
										taintedVars[formatExpr(sel)] = true
										taintedFields[sel.Sel.Name] = true
										funcHasSecret[fnName] = true
										if recvIdent, ok := sel.X.(*ast.Ident); ok {
											taintedVars[recvIdent.Name] = true
											taintedReceivers[recvIdent.Name] = true
										}
									}
									if idx, ok := lhs.(*ast.IndexExpr); ok {
										taintedVars[formatExpr(idx)] = true
										funcHasSecret[fnName] = true
										if mapIdent, ok := idx.X.(*ast.Ident); ok {
											taintedVars[mapIdent.Name] = true
										}
									}
								}
								if i < len(assign.Lhs) {
									if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
										if rhsIdent, ok := rhs.(*ast.Ident); ok {
											aliasMap[lhsIdent.Name] = rhsIdent.Name
											if taintedVars[rhsIdent.Name] {
												taintedVars[lhsIdent.Name] = true
											}
											if dsnExternalMap[rhsIdent.Name] {
												dsnExternalMap[lhsIdent.Name] = true
											}
										}
										// Pointer assignment: p := &b
										if unary, ok := rhs.(*ast.UnaryExpr); ok && unary.Op == token.AND {
											if targetIdent, ok := unary.X.(*ast.Ident); ok {
												ptrAliasMap[lhsIdent.Name] = targetIdent.Name
												aliasMap[lhsIdent.Name] = targetIdent.Name
												if taintedVars[targetIdent.Name] {
													taintedVars[lhsIdent.Name] = true
												}
											}
										}
									}
								}
							}
						}
						if call, ok := in.(*ast.CallExpr); ok {
							if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
								hasSecretArg := false
								for _, arg := range call.Args {
									if isSecretExpr(arg) {
										hasSecretArg = true
										break
									}
								}
								if hasSecretArg {
									funcHasSecret[fnName] = true
									if recvIdent, ok := sel.X.(*ast.Ident); ok {
										taintedVars[recvIdent.Name] = true
										taintedReceivers[recvIdent.Name] = true
									}
									taintedFields[sel.Sel.Name] = true
								}
							}
						}
						if rStmt, ok := in.(*ast.RangeStmt); ok {
							if isSecretExpr(rStmt.X) {
								if valIdent, ok := rStmt.Value.(*ast.Ident); ok {
									taintedVars[valIdent.Name] = true
									funcHasSecret[fnName] = true
								}
								if keyIdent, ok := rStmt.Key.(*ast.Ident); ok && rStmt.Value == nil {
									taintedVars[keyIdent.Name] = true
									funcHasSecret[fnName] = true
								}
							}
						}
						if valSpec, ok := in.(*ast.ValueSpec); ok {
							for i, val := range valSpec.Values {
								if isSecretExpr(val) && i < len(valSpec.Names) {
									taintedVars[valSpec.Names[i].Name] = true
									funcHasSecret[fnName] = true
									if isExternalHostDSN(val) {
										dsnExternalMap[valSpec.Names[i].Name] = true
									}
								}
							}
						}
						if ret, ok := in.(*ast.ReturnStmt); ok {
							for _, result := range ret.Results {
								if isSecretExpr(result) {
									taintedFuncs[fnName] = true
									funcHasSecret[fnName] = true
								}
							}
						}
						return true
					})
				}
			}
		}

		// Pass 2: Sinks and taboos inspection
		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				fnName := fn.Name.Name
				ast.Inspect(fn.Body, func(in ast.Node) bool {
					// 1. Binary expression self-comparison check
					if bin, ok := in.(*ast.BinaryExpr); ok {
						if bin.Op == token.NEQ || bin.Op == token.EQL {
							_, isXCall := bin.X.(*ast.CallExpr)
							_, isYCall := bin.Y.(*ast.CallExpr)
							if !isXCall && !isYCall {
								xStr := formatExpr(bin.X)
								yStr := formatExpr(bin.Y)
								xIdent, okX := bin.X.(*ast.Ident)
								yIdent, okY := bin.Y.(*ast.Ident)
								isAlias := false
								// Check pointer dereference *p vs b
								if starX, ok := bin.X.(*ast.StarExpr); ok {
									if ptrId, ok := starX.X.(*ast.Ident); ok {
										targetName := ptrAliasMap[ptrId.Name]
										if okY && targetName != "" && targetName == yIdent.Name {
											isAlias = true
										}
									}
								}
								if starY, ok := bin.Y.(*ast.StarExpr); ok {
									if ptrId, ok := starY.X.(*ast.Ident); ok {
										targetName := ptrAliasMap[ptrId.Name]
										if okX && targetName != "" && targetName == xIdent.Name {
											isAlias = true
										}
									}
								}
								if isAlias {
									violations = append(violations, fmt.Sprintf("test integrity violation: self-comparison tautology detected (%s %s %s)", xStr, bin.Op.String(), yStr))
								}
							}
						}
					}

					// 2. Empty range loop with assertions
					if rStmt, ok := in.(*ast.RangeStmt); ok {
						isEmpty := false
						if cl, ok := rStmt.X.(*ast.CompositeLit); ok {
							isFixedArray := false
							if arrType, ok := cl.Type.(*ast.ArrayType); ok && arrType.Len != nil {
								isFixedArray = true
							}
							if !isFixedArray && len(cl.Elts) == 0 {
								isEmpty = true
							}
						}
						if isEmpty && rStmt.Body != nil {
							ast.Inspect(rStmt.Body, func(bn ast.Node) bool {
								if call, ok := bn.(*ast.CallExpr); ok && isAssertionCall(call) {
									violations = append(violations, "test integrity violation: assertion inside empty range loop detected")
								}
								return true
							})
						}
					}

					if call, ok := in.(*ast.CallExpr); ok {
						callStr := formatExpr(call)
						funStr := formatExpr(call.Fun)
						funLower := strings.ToLower(funStr)

						// Collect function / method calls
						if id, ok := call.Fun.(*ast.Ident); ok {
							funcCalls[fnName] = append(funcCalls[fnName], id.Name)
						}
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							funcCalls[fnName] = append(funcCalls[fnName], sel.Sel.Name)
						}
						ast.Inspect(call.Fun, func(sub ast.Node) bool {
							if subCall, ok := sub.(*ast.CallExpr); ok {
								if subId, ok := subCall.Fun.(*ast.Ident); ok {
									funcCalls[fnName] = append(funcCalls[fnName], subId.Name)
								}
								if subSel, ok := subCall.Fun.(*ast.SelectorExpr); ok {
									funcCalls[fnName] = append(funcCalls[fnName], subSel.Sel.Name)
								}
							}
							return true
						})

						// Reflection call on process/network function
						if (strings.Contains(callStr, "reflect.ValueOf") || strings.Contains(funStr, "reflect.ValueOf")) &&
							(strings.Contains(callStr, "exec.Command") || strings.Contains(callStr, "syscall.Exec") ||
								strings.Contains(callStr, "http.Get") || strings.Contains(callStr, "http.Post") || strings.Contains(callStr, "net.Dial")) {
							violations = append(violations, "AST Taboo Violation: forbidden reflection call on process/network function")
						}

						// Sinks receiving secret/tainted arg
						isSink := false
						if funLower == "errors.new" || funLower == "fmt.errorf" ||
							strings.HasPrefix(funLower, "fmt.print") || strings.HasPrefix(funLower, "fmt.fprint") || strings.HasPrefix(funLower, "fmt.sprint") ||
							strings.HasPrefix(funLower, "log.print") || strings.HasPrefix(funLower, "log.fatal") ||
							funLower == "os.writefile" || funLower == "os.create" ||
							strings.HasPrefix(funLower, "http.") || strings.HasPrefix(funLower, "net.") ||
							strings.HasPrefix(funLower, "smtp.") || strings.HasPrefix(funLower, "rpc.") || strings.HasPrefix(funLower, "textproto.") ||
							funLower == "exec.command" || funLower == "exec.commandcontext" || funLower == "syscall.exec" {
							isSink = true
							funcHasSink[fnName] = true
						}

						if !isTestCode {
							if _, ok := call.Fun.(*ast.Ident); ok {
								for _, arg := range call.Args {
									if isSecretExpr(arg) {
										funcHasSecret[fnName] = true
										funcHasSink[fnName] = true
										violations = append(violations, "AST Taboo Violation: forbidden secret read reaching sink in non-test code")
									}
								}
							}

							if isSink {
								for _, arg := range call.Args {
									if isSecretExpr(arg) {
										funcHasSecret[fnName] = true
										violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden secret read reaching sink in non-test code (%s)", funStr))
									}
								}
							}

							if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
								sName := sel.Sel.Name
								pkgName := ""
								if pkgIdent, ok := sel.X.(*ast.Ident); ok {
									pkgName = pkgIdent.Name
								}

								// net/smtp sinks
								if pkgName == "smtp" || sName == "SendMail" || sName == "PlainAuth" || sName == "CRAMMD5Auth" ||
									sName == "Mail" || sName == "Rcpt" || sName == "Data" {
									funcHasNet[fnName] = true
									funcHasSink[fnName] = true
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											funcHasSecret[fnName] = true
											violations = append(violations, "AST Taboo Violation: forbidden secret exfiltration via net/smtp in non-test code")
										}
									}
								}

								// net/rpc sinks
								if pkgName == "rpc" || sName == "Call" || sName == "Go" || sName == "DialHTTP" || sName == "DialHTTPPath" {
									funcHasNet[fnName] = true
									funcHasSink[fnName] = true
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											funcHasSecret[fnName] = true
											violations = append(violations, "AST Taboo Violation: forbidden secret exfiltration via net/rpc in non-test code")
										}
									}
								}

								// net/textproto sinks
								if pkgName == "textproto" || sName == "Cmd" || sName == "PrintfLine" || sName == "DotWriter" {
									funcHasNet[fnName] = true
									funcHasSink[fnName] = true
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											funcHasSecret[fnName] = true
											violations = append(violations, "AST Taboo Violation: forbidden secret exfiltration via net/textproto in non-test code")
										}
									}
								}

								// database/sql external DSN sinks
								if (pkgName == "sql" || strings.HasPrefix(funLower, "sql.")) && (sName == "Open" || sName == "OpenDB") {
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											if isExternalHostDSN(arg) {
												funcHasNet[fnName] = true
												funcHasSink[fnName] = true
												funcHasSecret[fnName] = true
												violations = append(violations, "AST Taboo Violation: forbidden database/sql connection to remote host with secret in non-test code")
											}
										}
									}
								}

								// Network sinks
								if sName == "Post" || sName == "Get" || sName == "Do" || sName == "Dial" || sName == "NewRequest" || sName == "Head" ||
									sName == "LookupHost" || sName == "LookupIP" || sName == "LookupTXT" || sName == "LookupCNAME" || sName == "LookupAddr" {
									funcHasNet[fnName] = true
									funcHasSink[fnName] = true
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											violations = append(violations, "AST Taboo Violation: forbidden secret exfiltration / network egress in non-test code")
										}
									}
								}

								// Header and metadata sinks
								// Authorization Header Policy: Outbound HTTP requests setting standard "Authorization" or "Proxy-Authorization"
								// headers are permitted for authenticated API communication. Setting sensitive secrets on custom or metadata
								// headers (e.g. "X-Secret", "X-Custom-*") is treated as header exfiltration and rejected.
								if sName == "Set" || sName == "Add" {
									isAuthHeader := false
									if len(call.Args) >= 1 {
										if headerLit, ok := call.Args[0].(*ast.BasicLit); ok {
											headerName := strings.Trim(headerLit.Value, `"`)
											if strings.EqualFold(headerName, "Authorization") || strings.EqualFold(headerName, "Proxy-Authorization") {
												isAuthHeader = true
											}
										}
									}
									if !isAuthHeader {
										for _, arg := range call.Args {
											if isSecretExpr(arg) {
												funcHasSecret[fnName] = true
												funcHasSink[fnName] = true
												violations = append(violations, "AST Taboo Violation: forbidden secret read reaching Header/metadata sink in non-test code")
											}
										}
									}
								}

								// Process execution sinks
								if sName == "Command" || sName == "CommandContext" || sName == "Exec" {
									funcHasNet[fnName] = true
									funcHasSink[fnName] = true
								}

								// File write sinks
								if sName == "Write" || sName == "WriteFile" || sName == "Create" || sName == "OpenFile" || sName == "WriteString" {
									funcHasSink[fnName] = true
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											violations = append(violations, "AST Taboo Violation: forbidden secret write to file in non-test code")
										}
									}
								}

								// Output / stdout / log sinks
								if pkgName == "fmt" || pkgName == "log" || pkgName == "errors" {
									funcHasSink[fnName] = true
									for _, arg := range call.Args {
										if isSecretExpr(arg) {
											violations = append(violations, "AST Taboo Violation: forbidden secret read reaching sink in non-test code")
										}
									}
								}

								// Dynamic code loading & system calls
								if pkgName == "plugin" && sName == "Open" {
									violations = append(violations, "AST Taboo Violation: forbidden dynamic code loading via plugin.Open in non-test code")
								}
								if pkgName == "syscall" && (sName == "Exec" || sName == "ForkExec") {
									violations = append(violations, "AST Taboo Violation: forbidden low-level process execution via syscall.Exec in non-test code")
								}
								if pkgName == "C" && (sName == "system" || sName == "popen") {
									violations = append(violations, "AST Taboo Violation: forbidden cgo system execution (C.system)")
								}
								if sName == "Setenv" {
									for _, arg := range call.Args {
										if lit, ok := arg.(*ast.BasicLit); ok {
											v := strings.ToUpper(lit.Value)
											if strings.Contains(v, "LD_PRELOAD") || strings.Contains(v, "DYLD_INSERT_LIBRARIES") || strings.Contains(v, "LD_LIBRARY_PATH") {
												violations = append(violations, "AST Taboo Violation: forbidden dynamic linker environment variable modification (LD_PRELOAD)")
											}
										}
									}
								}
							}
						}
					}

					if !isTestCode {
						if cl, ok := in.(*ast.CompositeLit); ok {
							if isSecretExpr(cl) {
								typStr := strings.ToLower(formatExpr(cl.Type))
								if strings.Contains(typStr, "url.values") || strings.Contains(typStr, "header") || strings.Contains(typStr, "request") {
									funcHasSecret[fnName] = true
									funcHasSink[fnName] = true
									violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden secret read reaching network struct sink in non-test code (%s)", typStr))
								}
							}
						}
					}
					return true
				})
			}
		}

		if !isTestCode {
			// Check transitive secret exfiltration / sink reachability from any non-test function
			for fnName := range funcCalls {
				secretReachable := transitivelyHasProp(fnName, funcCalls, funcHasSecret, make(map[string]bool))
				sinkReachable := transitivelyHasProp(fnName, funcCalls, funcHasSink, make(map[string]bool))
				netReachable := transitivelyHasProp(fnName, funcCalls, funcHasNet, make(map[string]bool))
				if secretReachable && (sinkReachable || netReachable) {
					violations = append(violations, "AST Taboo Violation: forbidden secret exfiltration / secret read reaching sink in non-test code")
				}
			}
		}

		ast.Inspect(node, func(n ast.Node) bool {
			// 0. Direct call checks (e.g. dot-imported packages)
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok {
					if dotImports["os/exec"] && (id.Name == "Command" || id.Name == "CommandContext") {
						violations = append(violations, "AST Taboo Violation: forbidden process execution via dot-imported Command")
					}
					if dotImports["reflect"] && (id.Name == "ValueOf" || id.Name == "TypeOf" || id.Name == "MethodByName") {
						violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden reflection call via dot-imported reflect (%s)", id.Name))
					}
				}
			}

			// 1. Selector expressions: DefaultClient, DefaultServeMux, DefaultTransport, or aliased taboo package usage
			if sel, ok := n.(*ast.SelectorExpr); ok {
				name := sel.Sel.Name
				if checkDefaultClient && name == "DefaultClient" {
					violations = append(violations, "AST Taboo Violation: forbidden reference to DefaultClient (net/http.DefaultClient)")
				}
				if checkDefaultServeMux && name == "DefaultServeMux" {
					violations = append(violations, "AST Taboo Violation: forbidden reference to DefaultServeMux (net/http.DefaultServeMux)")
				}
				if checkDefaultTransport && name == "DefaultTransport" {
					violations = append(violations, "AST Taboo Violation: forbidden reference to DefaultTransport (net/http.DefaultTransport)")
				}

				// Check reflection calls
				if checkReflection {
					if id, ok := sel.X.(*ast.Ident); ok && (id.Name == "reflect" || importAliases[id.Name] == "reflect") {
						violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden reflection call (reflect.%s)", name))
					}
					if name == "MethodByName" || name == "ValueOf" || name == "TypeOf" || name == "New" || name == "MakeFunc" {
						violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden reflection invocation (%s)", name))
					}
				}

				// Check if selector target is an aliased taboo package
				if id, ok := sel.X.(*ast.Ident); ok {
					if pkgPath, exists := importAliases[id.Name]; exists {
						for _, taboo := range taboos {
							tLower := strings.ToLower(taboo)
							if strings.Contains(tLower, strings.ToLower(pkgPath)) || (pkgPath == "net/http" && strings.Contains(tLower, "net/http")) || (pkgPath == "os/exec" && (strings.Contains(tLower, "exec") || strings.Contains(tLower, "os/exec"))) || (pkgPath == "reflect" && (tLower == "reflect" || tLower == "reflection")) {
								violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden reference to taboo package %q via alias %s.%s", pkgPath, id.Name, name))
							}
						}
					}
				}
			}

			// 2. Insecure TLS: InsecureSkipVerify: true
			if kv, ok := n.(*ast.KeyValueExpr); ok && checkInsecureTLS {
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "InsecureSkipVerify" {
					if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
						violations = append(violations, "AST Taboo Violation: forbidden insecure TLS configuration (InsecureSkipVerify: true)")
					}
				}
			}

			// 3. Command execution with shell or remote push: exec.Command("sh", ...) or exec.Command("git", "push", ...)
			if call, ok := n.(*ast.CallExpr); ok && checkShellExec {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
					if len(call.Args) > 0 {
						argIdx := 0
						if sel.Sel.Name == "CommandContext" && len(call.Args) > 1 {
							argIdx = 1
						}
						prog := resolveStringExpr(call.Args[argIdx])
						if prog == "" {
							if lit, ok := call.Args[argIdx].(*ast.BasicLit); ok {
								prog = strings.Trim(lit.Value, `"`)
							}
						}
						progLower := strings.ToLower(prog)
						if progLower == "sh" || progLower == "bash" || progLower == "zsh" || progLower == "/bin/sh" || progLower == "/bin/bash" || progLower == "/bin/zsh" {
							violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden shell interpreter invocation via exec.Command(%q)", prog))
						}
						if progLower == "git" && len(call.Args) > argIdx+1 {
							arg1S := resolveStringExpr(call.Args[argIdx+1])
							if strings.Contains(arg1S, "push") {
								violations = append(violations, "AST Taboo Violation: forbidden remote git push via exec.Command")
							}
						}
						if progLower == "curl" || progLower == "wget" || progLower == "nc" {
							violations = append(violations, fmt.Sprintf("AST Taboo Violation: forbidden network fetch invocation via exec.Command(%q)", prog))
						}
					}
				}
			}

			return true
		})
	}

	if !isTestCode {
		// Specific system/process taboo checks on code blob
		codeLower := strings.ToLower(code)
		if strings.Contains(codeLower, "os.readfile") && strings.Contains(codeLower, ".aws/credentials") {
			violations = append(violations, "AST Taboo Violation: forbidden direct read of credential file in non-test code")
		}
		if strings.Contains(codeLower, "plugin.open(") {
			violations = append(violations, "AST Taboo Violation: forbidden dynamic code loading via plugin.Open in non-test code")
		}
		if strings.Contains(codeLower, "syscall.exec(") {
			violations = append(violations, "AST Taboo Violation: forbidden low-level process execution via syscall.Exec in non-test code")
		}
		if strings.Contains(codeLower, "c.system(") || strings.Contains(codeLower, "c.system (") {
			violations = append(violations, "AST Taboo Violation: forbidden cgo system execution (C.system)")
		}
		if strings.Contains(codeLower, "ld_preload") && strings.Contains(codeLower, "setenv") {
			violations = append(violations, "AST Taboo Violation: forbidden dynamic linker environment variable modification (LD_PRELOAD)")
		}
		if strings.Contains(codeLower, "exec.command(\"git\"") && strings.Contains(codeLower, "\"push\"") {
			violations = append(violations, "AST Taboo Violation: forbidden remote git push via exec.Command")
		}
		if strings.Contains(codeLower, "reflect.valueof") && (strings.Contains(codeLower, "exec.command") || strings.Contains(codeLower, "syscall.exec") || strings.Contains(codeLower, "http.get") || strings.Contains(codeLower, "http.post")) {
			violations = append(violations, "AST Taboo Violation: forbidden reflection call on process/network function")
		}
	}


	return deduplicateStrings(violations)
}

func inspectMultiLanguageSemantics(code string, taboos []string) []string {
	var violations []string
	diffLower := strings.ToLower(code)

	// Inherent security taboos across all languages: arbitrary command execution, destructive commands, file deletions on system paths
	if strings.Contains(diffLower, "runtime.getruntime().exec") ||
		strings.Contains(diffLower, "runtime.exec") ||
		strings.Contains(diffLower, "processbuilder") ||
		strings.Contains(diffLower, "rm -rf") ||
		strings.Contains(diffLower, "child_process.exec") ||
		strings.Contains(diffLower, "child_process.execsync") ||
		strings.Contains(diffLower, "execsync(") ||
		strings.Contains(diffLower, "os.system(") ||
		strings.Contains(diffLower, "os.system \"") ||
		strings.Contains(diffLower, "subprocess.call") ||
		strings.Contains(diffLower, "subprocess.popen") ||
		strings.Contains(diffLower, "subprocess.run") {
		violations = append(violations, "Security Taboo: forbidden arbitrary shell/command execution detected")
	}
	if (strings.Contains(diffLower, "process()") || strings.Contains(diffLower, "nstask()")) && (strings.Contains(diffLower, "/bin/sh") || strings.Contains(diffLower, "/bin/bash") || strings.Contains(diffLower, "executableurl") || strings.Contains(diffLower, "launchpath") || strings.Contains(diffLower, "fileurlwithpath") || strings.Contains(diffLower, "launch()")) {
		violations = append(violations, "Security Taboo: forbidden arbitrary shell/command execution detected in Swift (Process/NSTask)")
	}
	if strings.Contains(diffLower, "nstask()") || strings.Contains(diffLower, "nstask.launchedtask") {
		violations = append(violations, "Security Taboo: forbidden arbitrary process execution detected in Swift (NSTask)")
	}
	if strings.Contains(diffLower, "new function(") || strings.Contains(diffLower, "new function \"") || strings.Contains(diffLower, "new function('") || strings.Contains(diffLower, "new function(`") {
		violations = append(violations, "Security Taboo: forbidden dynamic code execution in TypeScript/JavaScript (new Function)")
	}
	if (strings.Contains(diffLower, "require('child_'") || strings.Contains(diffLower, "require(\"child_\"")) && (strings.Contains(diffLower, "'process'") || strings.Contains(diffLower, "\"process\"")) {
		violations = append(violations, "Security Taboo: forbidden arbitrary command execution in TypeScript/JavaScript (child_process)")
	}
	if (strings.Contains(diffLower, "files.delete") || strings.Contains(diffLower, "java.io.file") || strings.Contains(diffLower, "java.nio.file")) && (strings.Contains(diffLower, "/etc/") || strings.Contains(diffLower, "paths.get") || strings.Contains(diffLower, "/var/run") || strings.Contains(diffLower, "/root")) {
		violations = append(violations, "Security Taboo: forbidden destructive system file deletion detected")
	}

	// Normalized de-concatenated code to defeat string splitting: "java.lang." + "Runtime" -> java.lang.runtime
	reConcat := regexp.MustCompile(`["']\s*\+\s*["']`)
	deconcatCode := reConcat.ReplaceAllString(diffLower, "")

	// Python alias & reflection bypasses
	rePyAlias := regexp.MustCompile(`(?m)^\s*import\s+subprocess\s+as\s+(\w+)`)
	if m := rePyAlias.FindStringSubmatch(code); len(m) > 1 {
		alias := m[1]
		if strings.Contains(diffLower, alias+".") {
			violations = append(violations, fmt.Sprintf("Security Taboo: forbidden arbitrary shell/command execution detected (Python subprocess alias %s)", alias))
		}
	}
	rePyFromAlias := regexp.MustCompile(`(?m)^\s*from\s+os\s+import\s+system\s+as\s+(\w+)`)
	if m := rePyFromAlias.FindStringSubmatch(code); len(m) > 1 {
		alias := m[1]
		if strings.Contains(diffLower, alias+"(") {
			violations = append(violations, fmt.Sprintf("Security Taboo: forbidden arbitrary shell/command execution detected (Python os.system alias %s)", alias))
		}
	}
	if strings.Contains(diffLower, "getattr") && (strings.Contains(diffLower, "os") || strings.Contains(diffLower, "sys")) && (strings.Contains(diffLower, "system") || strings.Contains(diffLower, "popen") || strings.Contains(diffLower, `"sys"`) || strings.Contains(diffLower, `'sys'`)) {
		violations = append(violations, "Security Taboo: forbidden reflection/dynamic invocation (getattr)")
	}

	// Kotlin & Java import alias bypass: import java.lang.Runtime as EvilRuntime
	reKtImportAlias := regexp.MustCompile(`(?m)^\s*import\s+([\w\.]+)\s+as\s+(\w+)`)
	for _, m := range reKtImportAlias.FindAllStringSubmatch(code, -1) {
		if len(m) > 2 {
			pkg := strings.ToLower(m[1])
			alias := m[2]
			if strings.Contains(pkg, "runtime") || strings.Contains(pkg, "processbuilder") || strings.Contains(pkg, "process") {
				if strings.Contains(diffLower, strings.ToLower(alias)+".") || strings.Contains(diffLower, strings.ToLower(alias)+"(") {
					violations = append(violations, fmt.Sprintf("Security Taboo: forbidden arbitrary execution detected (Kotlin import alias %s -> %s)", alias, m[1]))
				}
			}
		}
	}

	// Kotlin & Swift typealias: typealias PB = java.lang.ProcessBuilder or typealias TaskRunner = Process
	reTypealias := regexp.MustCompile(`(?m)^\s*typealias\s+(\w+)\s*=\s*([\w\.]+)`)
	for _, m := range reTypealias.FindAllStringSubmatch(code, -1) {
		if len(m) > 2 {
			alias := m[1]
			target := strings.ToLower(m[2])
			if strings.Contains(target, "runtime") || strings.Contains(target, "processbuilder") || strings.Contains(target, "process") || strings.Contains(target, "nstask") {
				if strings.Contains(diffLower, strings.ToLower(alias)+".") || strings.Contains(diffLower, strings.ToLower(alias)+"(") || strings.Contains(diffLower, strings.ToLower(alias)+"()") {
					violations = append(violations, fmt.Sprintf("Security Taboo: forbidden execution detected via typealias %s = %s", alias, m[2]))
				}
			}
		}
	}

	// Kotlin / Java Reflection: Class.forName(...)
	if strings.Contains(deconcatCode, "class.forname") {
		if strings.Contains(deconcatCode, "runtime") || strings.Contains(deconcatCode, "processbuilder") || strings.Contains(deconcatCode, "process") {
			violations = append(violations, "Security Taboo: forbidden reflection Class.forName invocation of process execution class")
		}
	}

	// Swift Dynamic Loading / Reflection: NSClassFromString, objc_getClass
	if strings.Contains(deconcatCode, "nsclassfromstring") || strings.Contains(deconcatCode, "objc_getclass") {
		if strings.Contains(deconcatCode, "nstask") || strings.Contains(deconcatCode, "process") {
			violations = append(violations, "Security Taboo: forbidden dynamic class resolution (NSClassFromString/objc_getClass) for Process/NSTask")
		}
	}

	// JS/TS alias & concat bypasses
	reJSAlias := regexp.MustCompile(`import\s+[*]\s+as\s+(\w+)\s+from\s+["']child_process["']`)
	if m := reJSAlias.FindStringSubmatch(code); len(m) > 1 {
		alias := m[1]
		if strings.Contains(diffLower, alias+".") {
			violations = append(violations, fmt.Sprintf("Security Taboo: forbidden arbitrary command execution in TypeScript/JavaScript (child_process alias %s)", alias))
		}
	}
	if strings.Contains(diffLower, "require(") && (strings.Contains(diffLower, `"child"`) || strings.Contains(diffLower, `'child'`)) && (strings.Contains(diffLower, `"_process"`) || strings.Contains(diffLower, `'_process'`)) {
		violations = append(violations, "Security Taboo: forbidden arbitrary command execution in TypeScript/JavaScript (child_process require concatenation)")
	}

	for _, taboo := range tabooListCanonical(taboos) {
		lowerTaboo := strings.ToLower(strings.TrimSpace(taboo))
		if lowerTaboo == "" {
			continue
		}

		matched := false

		// Kotlin / Android
		if strings.Contains(lowerTaboo, "raw sqlite") && (strings.Contains(diffLower, "android.database.sqlite") || strings.Contains(diffLower, "sqlitedatabase") || strings.Contains(diffLower, "rawquery") || strings.Contains(deconcatCode, "sqlitedatabase")) {
			matched = true
		} else if strings.Contains(lowerTaboo, "blocking main thread") && (strings.Contains(diffLower, "thread.sleep") || strings.Contains(diffLower, "runblocking") || strings.Contains(diffLower, "dispatchers.main")) {
			matched = true
		} else if (strings.Contains(lowerTaboo, "runtime.exec") || strings.Contains(lowerTaboo, "exec") || strings.Contains(lowerTaboo, "shell") || strings.Contains(lowerTaboo, "processbuilder")) && (strings.Contains(diffLower, "runtime.getruntime().exec") || strings.Contains(diffLower, "runtime.exec") || strings.Contains(diffLower, "processbuilder") || strings.Contains(deconcatCode, "processbuilder") || strings.Contains(deconcatCode, "runtime.getruntime")) {
			matched = true
		}

		// Swift / macOS / iOS
		if strings.Contains(lowerTaboo, "process()") && (strings.Contains(diffLower, "process()") || strings.Contains(diffLower, "nstask") || strings.Contains(diffLower, "system(") || strings.Contains(deconcatCode, "nstask") || strings.Contains(deconcatCode, "process()")) {
			matched = true
		}

		// TypeScript / JavaScript
		if strings.Contains(lowerTaboo, "eval") && (strings.Contains(diffLower, "eval(") || strings.Contains(diffLower, "new function(") || strings.Contains(diffLower, "dangerouslysetinnerhtml")) {
			matched = true
		} else if strings.Contains(lowerTaboo, "child_process") && (strings.Contains(diffLower, "child_process.exec") || strings.Contains(diffLower, "child_process.spawn")) {
			matched = true
		}

		// Generic core token match
		if !matched {
			corePattern := extractCoreToken(lowerTaboo)
			if corePattern != "" && (strings.Contains(diffLower, corePattern) || strings.Contains(deconcatCode, corePattern)) {
				matched = true
			}
		}

		if matched {
			violations = append(violations, fmt.Sprintf("Violates Taboo: %s", taboo))
		}
	}

	return violations
}

func tabooListCanonical(in []string) []string {
	return in
}

func extractCoreToken(taboo string) string {
	cleaned := taboo
	prefixes := []string{"no ", "never ", "avoid ", "do not use ", "don't use ", "prohibit "}
	for _, p := range prefixes {
		if strings.HasPrefix(cleaned, p) {
			cleaned = strings.TrimPrefix(cleaned, p)
			break
		}
	}
	return strings.TrimSpace(cleaned)
}

func deduplicateStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] && s != "" {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// CheckTestIntegrityWholeFile performs post-patch whole-file AST analysis on test files.
func CheckTestIntegrityWholeFile(workspaceDir, diff string) []string {
	var violations []string

	// 1. Rename / deletion detection from diff
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "-") {
			trimmed := strings.TrimSpace(line[1:])
			if strings.HasPrefix(trimmed, "func Test") {
				// Check if subsequent addition is a rename to non-test
				for j := i + 1; j < len(lines) && j <= i+4; j++ {
					if strings.HasPrefix(lines[j], "+") {
						added := strings.TrimSpace(lines[j][1:])
						if strings.HasPrefix(added, "func ") && !strings.HasPrefix(added, "func Test") {
							violations = append(violations, fmt.Sprintf("test integrity violation: test %s renamed to non-test %s", trimmed, added))
						}
					}
				}
			}
		}
	}

	// 2. Discover test files touched
	touched := extractTouchedFiles(diff)
	testFiles := make(map[string]bool)
	for _, f := range touched {
		if strings.HasSuffix(f, "_test.go") {
			testFiles[f] = true
		}
	}
	// Also check if any _test.go exists directly in workspaceDir if touched didn't have paths
	if len(testFiles) == 0 && workspaceDir != "" {
		entries, _ := os.ReadDir(workspaceDir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), "_test.go") {
				testFiles[e.Name()] = true
			}
		}
	}

	fset := token.NewFileSet()
	for f := range testFiles {
		fullPath := f
		if workspaceDir != "" && !filepath.IsAbs(f) {
			fullPath = filepath.Join(workspaceDir, f)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		contentStr := string(data)
		if strings.Contains(contentStr, "//go:build ignore") || strings.Contains(contentStr, "+build ignore") || strings.Contains(contentStr, "//go:build exclude") {
			violations = append(violations, fmt.Sprintf("test integrity violation: test file %s contains build ignore tag", f))
		}
		node, err := parser.ParseFile(fset, fullPath, data, parser.ParseComments)
		if err != nil {
			continue
		}

		var helperFuncs []*ast.FuncDecl
		calledFuncs := make(map[string]bool)

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "Test") {
				// 2a. Early return at top of test
				if len(fn.Body.List) > 0 {
					firstStmt := fn.Body.List[0]
					if _, isRet := firstStmt.(*ast.ReturnStmt); isRet {
						violations = append(violations, fmt.Sprintf("test integrity violation: early return at top of test function %s in %s", fn.Name.Name, f))
					}
				}

				// Check any if true { return } or if os.Getenv("CI") == "" { return } in test body
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if ifStmt, ok := n.(*ast.IfStmt); ok {
						hasReturn := false
						for _, stmt := range ifStmt.Body.List {
							if _, isRet := stmt.(*ast.ReturnStmt); isRet {
								hasReturn = true
								break
							}
						}
						if hasReturn {
							if ident, ok := ifStmt.Cond.(*ast.Ident); ok && ident.Name == "true" {
								violations = append(violations, fmt.Sprintf("test integrity violation: if true { return } evasion in %s in %s", fn.Name.Name, f))
							}
							ast.Inspect(ifStmt.Cond, func(cn ast.Node) bool {
								if call, ok := cn.(*ast.CallExpr); ok {
									if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Getenv" {
										if len(call.Args) > 0 {
											if lit, ok := call.Args[0].(*ast.BasicLit); ok {
												envName := strings.ToUpper(strings.Trim(lit.Value, `"`))
												if envName == "CI" || envName == "CONTINUOUS_INTEGRATION" || envName == "TEST_RUN" {
													violations = append(violations, fmt.Sprintf("test integrity violation: if os.Getenv(%q) early return evasion in %s in %s", lit.Value, fn.Name.Name, f))
												}
											}
										}
									}
								}
								return true
							})
						}
					}
					return true
				})

				// Track calls
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
							for _, arg := range call.Args {
								if lit, ok := arg.(*ast.FuncLit); ok {
									if lit.Body == nil || len(lit.Body.List) == 0 {
										violations = append(violations, fmt.Sprintf("test integrity violation: empty t.Run body in %s in %s", fn.Name.Name, f))
									} else {
										hasCallOrAssertion := false
										for _, stmt := range lit.Body.List {
											if _, isRet := stmt.(*ast.ReturnStmt); isRet {
												continue
											}
											ast.Inspect(stmt, func(sn ast.Node) bool {
												if _, isCall := sn.(*ast.CallExpr); isCall {
													hasCallOrAssertion = true
												}
												return true
											})
										}
										if !hasCallOrAssertion {
											violations = append(violations, fmt.Sprintf("test integrity violation: no-op t.Run body in %s in %s", fn.Name.Name, f))
										}
									}
								}
							}
						}
					}
					return true
				})
			} else if fn.Name.Name != "init" && !strings.HasPrefix(fn.Name.Name, "Benchmark") && !strings.HasPrefix(fn.Name.Name, "Example") && !strings.HasPrefix(fn.Name.Name, "Fuzz") && !strings.HasPrefix(fn.Name.Name, "benchmark") && !strings.HasPrefix(fn.Name.Name, "fuzz") && !strings.HasPrefix(fn.Name.Name, "example") && !strings.HasPrefix(fn.Name.Name, "bm") && !strings.Contains(f, "bench") && !strings.Contains(f, "example") && !strings.Contains(f, "fuzz") && !strings.Contains(f, "timing") && !strings.Contains(f, "_testlog") {
				helperFuncs = append(helperFuncs, fn)
			}
		}

		// Track all identifier references across the file
		ast.Inspect(node, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				calledFuncs[ident.Name] = true
			}
			return true
		})

		// 2c. Check helpers with assertions that are never referenced
		for _, helper := range helperFuncs {
			hasAssertion := false
			ast.Inspect(helper.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						name := strings.ToLower(sel.Sel.Name)
						if name == "fatal" || name == "error" || name == "fail" || strings.HasPrefix(name, "assert") || strings.HasPrefix(name, "require") {
							hasAssertion = true
						}
					}
				}
				return true
			})
			// Check if helper is called from a test function
			isCalledFromTest := false
			for _, decl := range node.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && strings.HasPrefix(fn.Name.Name, "Test") {
					ast.Inspect(fn.Body, func(in ast.Node) bool {
						if ident, ok := in.(*ast.Ident); ok && ident.Name == helper.Name.Name {
							isCalledFromTest = true
						}
						return true
					})
				}
			}
			if hasAssertion && !isCalledFromTest && !calledFuncs[helper.Name.Name] {
				violations = append(violations, fmt.Sprintf("test integrity violation: assertions hidden in uncalled helper function %s in %s", helper.Name.Name, f))
			}
		}

		// Run AST test reachability analysis
		violations = append(violations, inspectTestASTIntegrity(node)...)
	}

	return deduplicateStrings(violations)
}

type constKind int

const (
	kindUnknown constKind = iota
	kindInt
	kindBool
	kindString
	kindNil
	kindAlias
)

type constResult struct {
	kind constKind
	iVal int64
	bVal bool
	sVal string
}

func resolveAlias(name string, vars map[string]constResult) string {
	visited := make(map[string]bool)
	curr := name
	for {
		if visited[curr] {
			return curr
		}
		visited[curr] = true
		if vars != nil {
			if res, ok := vars[curr]; ok && res.kind == kindAlias {
				curr = res.sVal
				continue
			}
		}
		return curr
	}
}

func evalConstExpr(expr ast.Expr, vars map[string]constResult, unsignedVars map[string]bool) (constResult, bool) {
	if expr == nil {
		return constResult{kind: kindUnknown}, false
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			v, err := strconv.ParseInt(e.Value, 0, 64)
			if err == nil {
				return constResult{kind: kindInt, iVal: v}, true
			}
			u, err := strconv.ParseUint(e.Value, 0, 64)
			if err == nil {
				return constResult{kind: kindInt, iVal: int64(u)}, true
			}
		case token.STRING:
			v, err := strconv.Unquote(e.Value)
			if err == nil {
				return constResult{kind: kindString, sVal: v}, true
			}
			return constResult{kind: kindString, sVal: strings.Trim(e.Value, `"`)}, true
		case token.CHAR:
			if len(e.Value) > 0 {
				return constResult{kind: kindInt, iVal: int64(e.Value[0])}, true
			}
		}
	case *ast.Ident:
		if e.Name == "true" {
			return constResult{kind: kindBool, bVal: true}, true
		}
		if e.Name == "false" {
			return constResult{kind: kindBool, bVal: false}, true
		}
		if e.Name == "nil" {
			return constResult{kind: kindNil}, true
		}
		if vars != nil {
			if res, ok := vars[e.Name]; ok {
				return res, true
			}
		}
	case *ast.ParenExpr:
		return evalConstExpr(e.X, vars, unsignedVars)
	case *ast.UnaryExpr:
		res, ok := evalConstExpr(e.X, vars, unsignedVars)
		if ok {
			switch e.Op {
			case token.NOT:
				if res.kind == kindBool {
					return constResult{kind: kindBool, bVal: !res.bVal}, true
				}
			case token.SUB:
				if res.kind == kindInt {
					return constResult{kind: kindInt, iVal: -res.iVal}, true
				}
			case token.ADD:
				if res.kind == kindInt {
					return constResult{kind: kindInt, iVal: res.iVal}, true
				}
			case token.XOR:
				if res.kind == kindInt {
					return constResult{kind: kindInt, iVal: ^res.iVal}, true
				}
			}
		}
	case *ast.CallExpr:
		funStr := formatExpr(e.Fun)
		if funStr == "len" && len(e.Args) == 1 {
			if strRes, ok := evalConstExpr(e.Args[0], vars, unsignedVars); ok && strRes.kind == kindString {
				return constResult{kind: kindInt, iVal: int64(len(strRes.sVal))}, true
			}
			if cl, ok := e.Args[0].(*ast.CompositeLit); ok {
				return constResult{kind: kindInt, iVal: int64(len(cl.Elts))}, true
			}
		}
	case *ast.BinaryExpr:
		if e.Op == token.LAND {
			rx, okX := evalConstExpr(e.X, vars, unsignedVars)
			if okX && rx.kind == kindBool && !rx.bVal {
				return constResult{kind: kindBool, bVal: false}, true
			}
			ry, okY := evalConstExpr(e.Y, vars, unsignedVars)
			if okY && ry.kind == kindBool && !ry.bVal {
				return constResult{kind: kindBool, bVal: false}, true
			}
			if okX && okY && rx.kind == kindBool && ry.kind == kindBool {
				return constResult{kind: kindBool, bVal: rx.bVal && ry.bVal}, true
			}
			if isContradictoryCondition(e.X, e.Y) {
				return constResult{kind: kindBool, bVal: false}, true
			}
		}
		if e.Op == token.LOR {
			rx, okX := evalConstExpr(e.X, vars, unsignedVars)
			if okX && rx.kind == kindBool && rx.bVal {
				return constResult{kind: kindBool, bVal: true}, true
			}
			ry, okY := evalConstExpr(e.Y, vars, unsignedVars)
			if okY && ry.kind == kindBool && ry.bVal {
				return constResult{kind: kindBool, bVal: true}, true
			}
			if okX && okY && rx.kind == kindBool && ry.kind == kindBool {
				return constResult{kind: kindBool, bVal: rx.bVal || ry.bVal}, true
			}
		}

		if e.Op == token.LSS {
			if id, ok := e.X.(*ast.Ident); ok && unsignedVars != nil && unsignedVars[id.Name] {
				ry, okY := evalConstExpr(e.Y, vars, unsignedVars)
				if okY && ry.kind == kindInt && ry.iVal <= 0 {
					return constResult{kind: kindBool, bVal: false}, true
				}
			}
		}

		rx, okX := evalConstExpr(e.X, vars, unsignedVars)
		ry, okY := evalConstExpr(e.Y, vars, unsignedVars)
		if okX && okY {
			if rx.kind == kindInt && ry.kind == kindInt {
				switch e.Op {
				case token.EQL:
					return constResult{kind: kindBool, bVal: rx.iVal == ry.iVal}, true
				case token.NEQ:
					return constResult{kind: kindBool, bVal: rx.iVal != ry.iVal}, true
				case token.LSS:
					return constResult{kind: kindBool, bVal: rx.iVal < ry.iVal}, true
				case token.LEQ:
					return constResult{kind: kindBool, bVal: rx.iVal <= ry.iVal}, true
				case token.GTR:
					return constResult{kind: kindBool, bVal: rx.iVal > ry.iVal}, true
				case token.GEQ:
					return constResult{kind: kindBool, bVal: rx.iVal >= ry.iVal}, true
				case token.ADD:
					return constResult{kind: kindInt, iVal: rx.iVal + ry.iVal}, true
				case token.SUB:
					return constResult{kind: kindInt, iVal: rx.iVal - ry.iVal}, true
				case token.MUL:
					return constResult{kind: kindInt, iVal: rx.iVal * ry.iVal}, true
				case token.QUO:
					if ry.iVal != 0 {
						return constResult{kind: kindInt, iVal: rx.iVal / ry.iVal}, true
					}
				case token.REM:
					if ry.iVal != 0 {
						return constResult{kind: kindInt, iVal: rx.iVal % ry.iVal}, true
					}
				case token.AND:
					return constResult{kind: kindInt, iVal: rx.iVal & ry.iVal}, true
				case token.OR:
					return constResult{kind: kindInt, iVal: rx.iVal | ry.iVal}, true
				case token.XOR:
					return constResult{kind: kindInt, iVal: rx.iVal ^ ry.iVal}, true
				case token.SHL:
					return constResult{kind: kindInt, iVal: rx.iVal << ry.iVal}, true
				case token.SHR:
					return constResult{kind: kindInt, iVal: rx.iVal >> ry.iVal}, true
				}
			}
			if rx.kind == kindString && ry.kind == kindString {
				switch e.Op {
				case token.EQL:
					return constResult{kind: kindBool, bVal: rx.sVal == ry.sVal}, true
				case token.NEQ:
					return constResult{kind: kindBool, bVal: rx.sVal != ry.sVal}, true
				}
			}
			if rx.kind == kindBool && ry.kind == kindBool {
				switch e.Op {
				case token.EQL:
					return constResult{kind: kindBool, bVal: rx.bVal == ry.bVal}, true
				case token.NEQ:
					return constResult{kind: kindBool, bVal: rx.bVal != ry.bVal}, true
				}
			}
			if rx.kind == kindNil && ry.kind == kindNil {
				switch e.Op {
				case token.EQL:
					return constResult{kind: kindBool, bVal: true}, true
				case token.NEQ:
					return constResult{kind: kindBool, bVal: false}, true
				}
			}
		}
		if idX, okX := e.X.(*ast.Ident); okX {
			if idY, okY := e.Y.(*ast.Ident); okY {
				nameX := resolveAlias(idX.Name, vars)
				nameY := resolveAlias(idY.Name, vars)
				if nameX == nameY && nameX != "nil" && nameX != "nan" && nameX != "f" && nameX != "err" && nameX != "v" &&
					nameX != "x" && nameX != "y" && nameX != "val" && nameX != "flt" && nameX != "fl" && nameX != "d" && nameX != "f32" && nameX != "f64" && nameX != "float" {
					if e.Op == token.EQL {
						return constResult{kind: kindBool, bVal: true}, true
					}
					if e.Op == token.NEQ {
						return constResult{kind: kindBool, bVal: false}, true
					}
				}
			}
		}
	}
	return constResult{kind: kindUnknown}, false
}

func isContradictoryCondition(x, y ast.Expr) bool {
	binX, okX := x.(*ast.BinaryExpr)
	binY, okY := y.(*ast.BinaryExpr)
	if !okX || !okY {
		return false
	}
	xLeft := formatExpr(binX.X)
	xRight := formatExpr(binX.Y)
	yLeft := formatExpr(binY.X)
	yRight := formatExpr(binY.Y)

	if (binX.Op == token.EQL && binY.Op == token.NEQ) || (binX.Op == token.NEQ && binY.Op == token.EQL) {
		if (xLeft == yLeft && xRight == yRight) || (xLeft == yRight && xRight == yLeft) {
			return true
		}
	}
	if binX.Op == token.EQL && binY.Op == token.EQL {
		if xLeft == yLeft && xRight != yRight {
			_, isLitX := binX.Y.(*ast.BasicLit)
			_, isLitY := binY.Y.(*ast.BasicLit)
			if isLitX && isLitY {
				return true
			}
		}
	}
	return false
}

func isTautologicalAssertion(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgName := ""
	if id, ok := sel.X.(*ast.Ident); ok {
		pkgName = id.Name
	}
	fnName := sel.Sel.Name
	isTestify := pkgName == "assert" || pkgName == "require"

	if isTestify {
		switch fnName {
		case "Equal", "Equalf", "Exactly", "Exactlyf", "Same", "Samef":
			if len(call.Args) >= 3 {
				aStr := formatExpr(call.Args[1])
				bStr := formatExpr(call.Args[2])
				if aStr != "" && aStr == bStr {
					return true
				}
			}
		case "True", "Truef":
			if len(call.Args) >= 2 {
				if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == "true" {
					return true
				}
			}
		case "False", "Falsef":
			if len(call.Args) >= 2 {
				if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == "false" {
					return true
				}
			}
		case "Nil", "Nilf", "NoError", "NoErrorf":
			if len(call.Args) >= 2 {
				if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == "nil" {
					return true
				}
			}
		case "Empty", "Emptyf":
			if len(call.Args) >= 2 {
				arg := call.Args[1]
				if id, ok := arg.(*ast.Ident); ok && id.Name == "nil" {
					return true
				}
				if lit, ok := arg.(*ast.BasicLit); ok && (lit.Value == `""` || lit.Value == "``") {
					return true
				}
				if cl, ok := arg.(*ast.CompositeLit); ok && len(cl.Elts) == 0 {
					return true
				}
			}
		case "Len", "Lenf":
			if len(call.Args) >= 3 {
				argObj := call.Args[1]
				argLen := call.Args[2]
				lenIsZero := false
				if lit, ok := argLen.(*ast.BasicLit); ok && lit.Kind == token.INT && lit.Value == "0" {
					lenIsZero = true
				}
				if lenIsZero {
					if lit, ok := argObj.(*ast.BasicLit); ok && (lit.Value == `""` || lit.Value == "``") {
						return true
					}
					if cl, ok := argObj.(*ast.CompositeLit); ok && len(cl.Elts) == 0 {
						return true
					}
				}
			}
		case "Contains", "Containsf":
			if len(call.Args) >= 3 {
				subArg := call.Args[2]
				if lit, ok := subArg.(*ast.BasicLit); ok && (lit.Value == `""` || lit.Value == "``") {
					return true
				}
			}
		}
	}
	return false
}

func isNonWorkTestCall(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		name := sel.Sel.Name
		if name == "Parallel" || name == "Setenv" || name == "Helper" || name == "Skip" || name == "Skipf" || name == "SkipNow" {
			return true
		}
	}
	return false
}

func isControlFlowTerminator(stmt ast.Stmt) bool {
	if stmt == nil {
		return false
	}
	if _, isRet := stmt.(*ast.ReturnStmt); isRet {
		return true
	}
	if exprStmt, ok := stmt.(*ast.ExprStmt); ok {
		if call, ok := exprStmt.X.(*ast.CallExpr); ok {
			funStr := formatExpr(call.Fun)
			if funStr == "runtime.Goexit" || funStr == "os.Exit" ||
				funStr == "panic" ||
				strings.HasPrefix(funStr, "log.Fatal") {
				return true
			}
		}
	}
	return false
}

func isGatedEarlyReturn(ifStmt *ast.IfStmt) bool {
	if ifStmt == nil {
		return false
	}
	hasReturn := false
	hasAssertionOrHelper := false
	ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
		if _, isRet := n.(*ast.ReturnStmt); isRet {
			hasReturn = true
		}
		if call, ok := n.(*ast.CallExpr); ok && isRealAssertionCall(call) {
			hasAssertionOrHelper = true
		}
		return true
	})
	if !hasReturn || hasAssertionOrHelper {
		return false
	}
	condStr := formatExpr(ifStmt.Cond)
	condLower := strings.ToLower(condStr)
	if strings.Contains(condLower, "go_want_helper_process") {
		return false
	}
	if strings.Contains(condLower, "runtime.goos") ||
		strings.Contains(condLower, "runtime.goarch") ||
		strings.Contains(condLower, "os.getenv") ||
		strings.Contains(condLower, "getenv") {
		return true
	}
	return false
}

func isRealAssertionCall(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if isNonWorkTestCall(call) || isTautologicalAssertion(call) {
		return false
	}
	// Passing *testing.T / *testing.B / t to any helper function indicates test logic
	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && (id.Name == "t" || id.Name == "b" || id.Name == "tb") {
			return true
		}
	}
	funExpr := call.Fun
	if idx, ok := funExpr.(*ast.IndexExpr); ok {
		funExpr = idx.X
	} else if idxList, ok := funExpr.(*ast.IndexListExpr); ok {
		funExpr = idxList.X
	}

	if sel, ok := funExpr.(*ast.SelectorExpr); ok {
		name := strings.ToLower(sel.Sel.Name)
		if name == "done" || name == "add" || name == "parallel" || name == "setenv" || name == "helper" ||
			name == "log" || name == "logf" || name == "tempdir" {
			return false
		}
		if name == "cleanup" {
			return true
		}
		if name == "fatal" || name == "fatalf" || name == "error" || name == "errorf" || name == "fail" || name == "failnow" ||
			strings.HasPrefix(name, "assert") || strings.HasPrefix(name, "require") ||
			strings.HasPrefix(name, "test") || strings.HasPrefix(name, "check") || strings.HasPrefix(name, "verify") ||
			strings.HasPrefix(name, "equal") || strings.HasPrefix(name, "is") || strings.HasPrefix(name, "match") ||
			strings.HasPrefix(name, "must") || strings.HasPrefix(name, "expect") || strings.HasPrefix(name, "validate") ||
			strings.HasPrefix(name, "diff") || strings.HasPrefix(name, "compare") || strings.HasPrefix(name, "free") ||
			strings.HasPrefix(name, "close") || strings.HasPrefix(name, "parse") || strings.HasPrefix(name, "execute") ||
			strings.HasPrefix(name, "templates") || strings.HasPrefix(name, "complete") || strings.HasPrefix(name, "nummethods") ||
			strings.HasPrefix(name, "new") || strings.HasPrefix(name, "clone") || strings.HasPrefix(name, "read") ||
			strings.HasPrefix(name, "write") || strings.HasPrefix(name, "lookup") ||
			strings.HasPrefix(name, "sort") || strings.HasPrefix(name, "exec") || strings.HasPrefix(name, "run") ||
			strings.HasPrefix(name, "do") || strings.HasPrefix(name, "send") || strings.HasPrefix(name, "recv") ||
			strings.HasPrefix(name, "stop") || strings.HasPrefix(name, "start") || strings.HasPrefix(name, "reset") ||
			strings.HasPrefix(name, "eval") || strings.HasPrefix(name, "compile") || strings.HasPrefix(name, "scan") ||
			strings.HasPrefix(name, "walk") || strings.HasPrefix(name, "iter") || strings.HasPrefix(name, "bench") ||
			strings.HasPrefix(name, "yield") || strings.HasPrefix(name, "after") || strings.HasPrefix(name, "calibrate") ||
			strings.HasPrefix(name, "nan") || strings.HasPrefix(name, "isnan") {
			return true
		}
	}
	if ident, ok := funExpr.(*ast.Ident); ok {
		name := strings.ToLower(ident.Name)
		if name == "panic" {
			return true
		}
		if strings.HasPrefix(name, "assert") || strings.HasPrefix(name, "require") || strings.HasPrefix(name, "equal") ||
			strings.HasPrefix(name, "test") || strings.HasPrefix(name, "check") || strings.HasPrefix(name, "verify") ||
			strings.HasPrefix(name, "chk") || strings.HasPrefix(name, "must") || strings.HasPrefix(name, "expect") ||
			strings.HasPrefix(name, "validate") || strings.HasPrefix(name, "match") || strings.HasPrefix(name, "new") ||
			strings.HasPrefix(name, "parse") || strings.HasPrefix(name, "read") || strings.HasPrefix(name, "write") ||
			strings.HasPrefix(name, "sort") || strings.HasPrefix(name, "exec") || strings.HasPrefix(name, "run") ||
			strings.HasPrefix(name, "do") || strings.HasPrefix(name, "send") || strings.HasPrefix(name, "recv") ||
			strings.HasPrefix(name, "stop") || strings.HasPrefix(name, "start") || strings.HasPrefix(name, "reset") ||
			strings.HasPrefix(name, "eval") || strings.HasPrefix(name, "compile") || strings.HasPrefix(name, "scan") ||
			strings.HasPrefix(name, "walk") || strings.HasPrefix(name, "iter") || strings.HasPrefix(name, "bench") ||
			strings.HasPrefix(name, "yield") || strings.HasPrefix(name, "after") || strings.HasPrefix(name, "calibrate") ||
			strings.HasPrefix(name, "nan") || strings.HasPrefix(name, "isnan") {
			return true
		}
	}
	return isAssertionCall(call)
}

func helperCanReachAssertions(helper *ast.FuncDecl, helpers map[string]*ast.FuncDecl, visited ...map[string]bool) bool {
	if helper == nil || helper.Body == nil {
		return false
	}
	var vMap map[string]bool
	if len(visited) > 0 && visited[0] != nil {
		vMap = visited[0]
	} else {
		vMap = make(map[string]bool)
	}
	if vMap[helper.Name.Name] {
		return false
	}
	vMap[helper.Name.Name] = true

	hasAssertion := false
	ast.Inspect(helper.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if isRealAssertionCall(call) {
				hasAssertion = true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok {
				if childHelper, okH := helpers[ident.Name]; okH && !vMap[ident.Name] {
					if helperCanReachAssertions(childHelper, helpers, vMap) {
						hasAssertion = true
					}
				}
			}
		}
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "Wait" || sel.Sel.Name == "Done" || sel.Sel.Name == "Go" {
				hasAssertion = true
			}
		}
		if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
			hasAssertion = true
		}
		return true
	})
	return hasAssertion
}

type syncJoinInfo struct {
	goroutineCount              int
	goroutineWithAssertionCount int
	errgroupGoCount             int
	hasWaitCall                 bool
	hasChanRecv                 bool
}

func cloneVars(vars map[string]constResult) map[string]constResult {
	if vars == nil {
		return nil
	}
	cp := make(map[string]constResult, len(vars))
	for k, v := range vars {
		cp[k] = v
	}
	return cp
}

func checkASTBlock(stmts []ast.Stmt, active bool, vars map[string]constResult, unsignedVars map[string]bool, helpers map[string]*ast.FuncDecl, closures map[string]*ast.FuncLit, calledClosures map[string]bool, outerJoins *syncJoinInfo) (foundAssertion bool, terminated bool, violations []string) {
	bypassedToLabel := ""
	for _, stmt := range stmts {
		if bypassedToLabel != "" {
			if lStmt, ok := stmt.(*ast.LabeledStmt); ok && lStmt.Label.Name == bypassedToLabel {
				bypassedToLabel = ""
				stmt = lStmt.Stmt
			} else {
				continue
			}
		}
		if !active {
			continue
		}

		if branch, ok := stmt.(*ast.BranchStmt); ok && branch.Tok == token.GOTO && branch.Label != nil {
			bypassedToLabel = branch.Label.Name
			continue
		}

		if isControlFlowTerminator(stmt) {
			terminated = true
			active = false
			continue
		}

		if exprStmt, ok := stmt.(*ast.ExprStmt); ok {
			if call, ok := exprStmt.X.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					name := sel.Sel.Name
					if name == "Skip" || name == "Skipf" || name == "SkipNow" {
						if !foundAssertion {
							terminated = true
							active = false
							continue
						}
					}
				}
			}
		}

		if selStmt, ok := stmt.(*ast.SelectStmt); ok {
			if selStmt.Body != nil {
				for _, commStmt := range selStmt.Body.List {
					if cc, ok := commStmt.(*ast.CommClause); ok {
						if cc.Comm == nil {
							// default: clause
							fAss, _, v := checkASTBlock(cc.Body, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
							if fAss {
								foundAssertion = true
							}
							violations = append(violations, v...)
							continue
						}
						isNilChan := false
						var chanExpr ast.Expr
						if exprStmt, ok := cc.Comm.(*ast.ExprStmt); ok {
							if unary, ok := exprStmt.X.(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
								chanExpr = unary.X
							}
						} else if assign, ok := cc.Comm.(*ast.AssignStmt); ok && len(assign.Rhs) > 0 {
							if unary, ok := assign.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
								chanExpr = unary.X
							}
						} else if send, ok := cc.Comm.(*ast.SendStmt); ok {
							chanExpr = send.Chan
						}

						if chanExpr != nil {
							exprToCheck := chanExpr
							if paren, ok := exprToCheck.(*ast.ParenExpr); ok {
								exprToCheck = paren.X
							}
							if call, ok := exprToCheck.(*ast.CallExpr); ok {
								funExpr := call.Fun
								if paren, ok := funExpr.(*ast.ParenExpr); ok {
									funExpr = paren.X
								}
								if _, isChan := funExpr.(*ast.ChanType); isChan && len(call.Args) == 1 {
									if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "nil" {
										isNilChan = true
									}
								}
							}
							if id, ok := exprToCheck.(*ast.Ident); ok && id.Name == "nil" {
								isNilChan = true
							}
						}

						if isNilChan {
							violations = append(violations, "test integrity violation: dead nil-channel select case detected")
						} else {
							foundAssertion = true
							fAss, _, v := checkASTBlock(cc.Body, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
							if fAss {
								foundAssertion = true
							}
							violations = append(violations, v...)
						}
					}
				}
			}
			continue
		}

		if defStmt, ok := stmt.(*ast.DeferStmt); ok {
			if defStmt.Call != nil {
				if fnLit, ok := defStmt.Call.Fun.(*ast.FuncLit); ok && fnLit.Body != nil {
					fAss, _, v := checkASTBlock(fnLit.Body.List, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
					if fAss {
						foundAssertion = true
					}
					violations = append(violations, v...)
				} else if isRealAssertionCall(defStmt.Call) {
					foundAssertion = true
				}
			}
			continue
		}

		if ifStmt, ok := stmt.(*ast.IfStmt); ok {
			condStr := formatExpr(ifStmt.Cond)
			if isGatedEarlyReturn(ifStmt) {
				violations = append(violations, fmt.Sprintf("test integrity violation: environment/OS/arch-gated early return detected (%s)", condStr))
			}
			if strings.Contains(condStr, "testing.Short()") || strings.Contains(condStr, "Short()") {
				foundAssertion = true
			}

			condRes, okCond := evalConstExpr(ifStmt.Cond, vars, unsignedVars)
			if okCond && condRes.kind == kindBool {
				if condRes.bVal {
					fAss, termThen, v := checkASTBlock(ifStmt.Body.List, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
					if fAss {
						foundAssertion = true
					}
					violations = append(violations, v...)
					if termThen {
						terminated = true
						active = false
					}
				} else {
					if ifStmt.Else != nil {
						var elseStmts []ast.Stmt
						if elseBlock, ok := ifStmt.Else.(*ast.BlockStmt); ok {
							elseStmts = elseBlock.List
						} else if elseIf, ok := ifStmt.Else.(*ast.IfStmt); ok {
							elseStmts = []ast.Stmt{elseIf}
						}
						fAss, termElse, v := checkASTBlock(elseStmts, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
						if fAss {
							foundAssertion = true
						}
						violations = append(violations, v...)
						if termElse {
							terminated = true
							active = false
						}
					}
				}
			} else {
				fThen, termThen, v1 := checkASTBlock(ifStmt.Body.List, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
				if fThen {
					foundAssertion = true
				}
				violations = append(violations, v1...)

				var fElse bool
				var termElse bool
				if ifStmt.Else != nil {
					var elseStmts []ast.Stmt
					if elseBlock, ok := ifStmt.Else.(*ast.BlockStmt); ok {
						elseStmts = elseBlock.List
					} else if elseIf, ok := ifStmt.Else.(*ast.IfStmt); ok {
						elseStmts = []ast.Stmt{elseIf}
					}
					var v2 []string
					fElse, termElse, v2 = checkASTBlock(elseStmts, active, cloneVars(vars), unsignedVars, helpers, closures, calledClosures, outerJoins)
					if fElse {
						foundAssertion = true
					}
					violations = append(violations, v2...)
				}
				if termThen && termElse {
					terminated = true
					active = false
				}
			}
			continue
		}

		if rStmt, ok := stmt.(*ast.RangeStmt); ok {
			isEmptyRange := false
			if lit, ok := rStmt.X.(*ast.BasicLit); ok && lit.Kind == token.INT && lit.Value == "0" {
				isEmptyRange = true
			} else if cl, ok := rStmt.X.(*ast.CompositeLit); ok && len(cl.Elts) == 0 {
				if arrTyp, ok := cl.Type.(*ast.ArrayType); ok && arrTyp.Len != nil {
					isEmptyRange = false
				} else {
					isEmptyRange = true
				}
			}
			if !isEmptyRange {
				loopVars := cloneVars(vars)
				ast.Inspect(rStmt.Body, func(n ast.Node) bool {
					if assign, ok := n.(*ast.AssignStmt); ok {
						for _, lhs := range assign.Lhs {
							if id, ok := lhs.(*ast.Ident); ok && loopVars != nil {
								delete(loopVars, id.Name)
							}
						}
					}
					if incDec, ok := n.(*ast.IncDecStmt); ok {
						if id, ok := incDec.X.(*ast.Ident); ok && loopVars != nil {
							delete(loopVars, id.Name)
						}
					}
					return true
				})
				fAss, _, v := checkASTBlock(rStmt.Body.List, active, loopVars, unsignedVars, helpers, closures, calledClosures, outerJoins)
				if fAss {
					foundAssertion = true
				}
				violations = append(violations, v...)
			}
			continue
		}

		if forStmt, ok := stmt.(*ast.ForStmt); ok {
			isDeadLoop := false
			if forStmt.Cond != nil {
				cRes, okC := evalConstExpr(forStmt.Cond, vars, unsignedVars)
				if okC && cRes.kind == kindBool && !cRes.bVal {
					isDeadLoop = true
				}
			}
			if !isDeadLoop && forStmt.Body != nil {
				loopVars := cloneVars(vars)
				ast.Inspect(forStmt.Body, func(n ast.Node) bool {
					if assign, ok := n.(*ast.AssignStmt); ok {
						for _, lhs := range assign.Lhs {
							if id, ok := lhs.(*ast.Ident); ok && loopVars != nil {
								delete(loopVars, id.Name)
							}
						}
					}
					if incDec, ok := n.(*ast.IncDecStmt); ok {
						if id, ok := incDec.X.(*ast.Ident); ok && loopVars != nil {
							delete(loopVars, id.Name)
						}
					}
					return true
				})
				fAss, _, v := checkASTBlock(forStmt.Body.List, active, loopVars, unsignedVars, helpers, closures, calledClosures, outerJoins)
				if fAss {
					foundAssertion = true
				}
				violations = append(violations, v...)
			}
			continue
		}

		if swStmt, ok := stmt.(*ast.SwitchStmt); ok {
			switchVars := cloneVars(vars)
			if swStmt.Init != nil {
				fAss, _, v := checkASTBlock([]ast.Stmt{swStmt.Init}, active, switchVars, unsignedVars, helpers, closures, calledClosures, outerJoins)
				if fAss {
					foundAssertion = true
				}
				violations = append(violations, v...)
			}
			if swStmt.Tag == nil {
				if swStmt.Body != nil {
					for _, clause := range swStmt.Body.List {
						if cc, ok := clause.(*ast.CaseClause); ok {
							caseActive := active
							if len(cc.List) > 0 {
								allConstFalse := true
								for _, expr := range cc.List {
									exprRes, okExpr := evalConstExpr(expr, switchVars, unsignedVars)
									if !okExpr || (exprRes.kind == kindBool && exprRes.bVal) {
										allConstFalse = false
										break
									}
								}
								if allConstFalse {
									caseActive = false
								}
							}
							if caseActive {
								fAss, _, v := checkASTBlock(cc.Body, true, cloneVars(switchVars), unsignedVars, helpers, closures, calledClosures, outerJoins)
								if fAss {
									foundAssertion = true
								}
								violations = append(violations, v...)
							}
						}
					}
				}
			} else {
				tagRes, okTag := evalConstExpr(swStmt.Tag, switchVars, unsignedVars)
				if swStmt.Body != nil {
					for _, clause := range swStmt.Body.List {
						if cc, ok := clause.(*ast.CaseClause); ok {
							caseActive := active
							if len(cc.List) > 0 && okTag {
								matchedAny := false
								hasDynamic := false
								for _, expr := range cc.List {
									exprRes, okExpr := evalConstExpr(expr, switchVars, unsignedVars)
									if !okExpr {
										hasDynamic = true
										break
									}
									if okExpr && exprRes == tagRes {
										matchedAny = true
										break
									}
								}
								if !matchedAny && !hasDynamic {
									caseActive = false
								}
							}
							if caseActive {
								fAss, _, v := checkASTBlock(cc.Body, true, cloneVars(switchVars), unsignedVars, helpers, closures, calledClosures, outerJoins)
								if fAss {
									foundAssertion = true
								}
								violations = append(violations, v...)
							}
						}
					}
				}
			}
			continue
		}

		if assign, ok := stmt.(*ast.AssignStmt); ok {
			for i, rhs := range assign.Rhs {
				if fnLit, ok := rhs.(*ast.FuncLit); ok {
					if i < len(assign.Lhs) {
						if id, ok := assign.Lhs[i].(*ast.Ident); ok && closures != nil {
							closures[id.Name] = fnLit
						}
					}
				}
				if sel, ok := rhs.(*ast.SelectorExpr); ok {
					if isRealAssertionCall(&ast.CallExpr{Fun: sel}) {
						if i < len(assign.Lhs) {
							if id, ok := assign.Lhs[i].(*ast.Ident); ok && vars != nil {
								vars[id.Name] = constResult{kind: kindString, sVal: "__assertion_fn__"}
							}
						}
					}
				}
				if i < len(assign.Lhs) {
					if lhsId, ok := assign.Lhs[i].(*ast.Ident); ok && vars != nil {
						if bRes, okB := evalConstExpr(rhs, vars, unsignedVars); okB && bRes.kind == kindBool {
							vars[lhsId.Name] = bRes
						} else if rhsId, okR := rhs.(*ast.Ident); okR && rhsId.Name != "nil" && rhsId.Name != "true" && rhsId.Name != "false" {
							vars[lhsId.Name] = constResult{kind: kindAlias, sVal: rhsId.Name}
						}
					}
				}
			}
		}

		if goStmt, ok := stmt.(*ast.GoStmt); ok {
			if outerJoins != nil {
				outerJoins.goroutineCount++
			}
			goroutineHasAssertion := false
			ast.Inspect(goStmt.Call, func(gn ast.Node) bool {
				if call, ok := gn.(*ast.CallExpr); ok && isRealAssertionCall(call) {
					goroutineHasAssertion = true
				}
				return true
			})
			if goroutineHasAssertion && outerJoins != nil {
				outerJoins.goroutineWithAssertionCount++
			}
			continue
		}

		ast.Inspect(stmt, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				for _, rhs := range assign.Rhs {
					if _, ok := rhs.(*ast.FuncLit); ok {
						return false
					}
				}
			}
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Run" {
						for _, arg := range call.Args {
							if fnLit, ok := arg.(*ast.FuncLit); ok && fnLit.Body != nil {
								if len(fnLit.Body.List) > 0 {
									if _, isRet := fnLit.Body.List[0].(*ast.ReturnStmt); isRet {
										violations = append(violations, "test integrity violation: early return before assertion in t.Run body")
										return true
									}
								}
								subAss, _, subV := checkASTBlock(fnLit.Body.List, true, vars, unsignedVars, helpers, closures, calledClosures, outerJoins)
								violations = append(violations, subV...)
								if subAss {
									foundAssertion = true
								} else {
									hasAnyWork := false
									ast.Inspect(fnLit.Body, func(sn ast.Node) bool {
										if subCall, ok := sn.(*ast.CallExpr); ok {
											if !isNonWorkTestCall(subCall) {
												subFunStr := formatExpr(subCall.Fun)
												if !strings.HasPrefix(subFunStr, "t.Log") && !strings.HasPrefix(subFunStr, "t.log") {
													hasAnyWork = true
												}
											}
										}
										return true
									})
									if hasAnyWork {
										foundAssertion = true
									} else {
										violations = append(violations, "test integrity violation: t.Run subtest has no reachable assertions")
									}
								}
							} else if _, ok := arg.(*ast.CallExpr); ok {
								foundAssertion = true
							} else if _, ok := arg.(*ast.Ident); ok {
								foundAssertion = true
							}
						}
						return false
					}
					if sel.Sel.Name == "Cleanup" {
						return false
					}
					if sel.Sel.Name == "Go" && outerJoins != nil {
						outerJoins.errgroupGoCount++
					}
					if sel.Sel.Name == "Wait" && outerJoins != nil {
						outerJoins.hasWaitCall = true
					}
				}

				for _, arg := range call.Args {
					if fnLit, ok := arg.(*ast.FuncLit); ok && fnLit.Body != nil {
						fAss, _, subV := checkASTBlock(fnLit.Body.List, true, vars, unsignedVars, helpers, closures, calledClosures, outerJoins)
						violations = append(violations, subV...)
						if fAss {
							foundAssertion = true
						}
					}
					if argId, ok := arg.(*ast.Ident); ok && closures != nil && closures[argId.Name] != nil {
						if !calledClosures[argId.Name] {
							calledClosures[argId.Name] = true
							fAss, _, subV := checkASTBlock(closures[argId.Name].Body.List, true, vars, unsignedVars, helpers, closures, calledClosures, outerJoins)
							violations = append(violations, subV...)
							if fAss {
								foundAssertion = true
							}
						}
					}
				}

				if ident, ok := call.Fun.(*ast.Ident); ok {
					if vars != nil && vars[ident.Name].sVal == "__assertion_fn__" {
						foundAssertion = true
					}
					if helper, ok := helpers[ident.Name]; ok {
						if helperCanReachAssertions(helper, helpers) {
							foundAssertion = true
						}
					} else if closures != nil && closures[ident.Name] != nil {
						if !calledClosures[ident.Name] {
							calledClosures[ident.Name] = true
							fAss, _, subV := checkASTBlock(closures[ident.Name].Body.List, true, vars, unsignedVars, helpers, closures, calledClosures, outerJoins)
							violations = append(violations, subV...)
							if fAss {
								foundAssertion = true
							}
						}
					} else if isRealAssertionCall(call) {
						foundAssertion = true
					}
				} else if isRealAssertionCall(call) {
					foundAssertion = true
				}
			}
			if unary, ok := n.(*ast.UnaryExpr); ok {
				if unary.Op == token.ARROW && outerJoins != nil {
					outerJoins.hasChanRecv = true
				}
				if unary.Op == token.AND {
					if id, ok := unary.X.(*ast.Ident); ok && vars != nil {
						delete(vars, id.Name)
					}
				}
			}
			return true
		})
	}
	return foundAssertion, terminated, violations
}

// inspectTestASTIntegrity performs comprehensive AST assertion reachability analysis on Go test files.
func inspectTestASTIntegrity(node *ast.File) []string {
	var violations []string
	if node == nil {
		return violations
	}

	// 1. Collect file-level constants
	fileConsts := make(map[string]constResult)
	for _, decl := range node.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.CONST {
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for i, name := range vs.Names {
						if i < len(vs.Values) {
							if res, ok := evalConstExpr(vs.Values[i], fileConsts, nil); ok {
								fileConsts[name.Name] = res
							}
						}
					}
				}
			}
		}
	}

	// 2. Collect helper functions and test functions
	helpers := make(map[string]*ast.FuncDecl)
	var testFuncs []*ast.FuncDecl
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if fn.Recv != nil {
			if !strings.HasPrefix(fn.Name.Name, "Benchmark") && !strings.HasPrefix(fn.Name.Name, "Example") {
				helpers[fn.Name.Name] = fn
			}
			continue
		}
		if fn.Name.Name == "TestMain" {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			firstParamType := formatExpr(fn.Type.Params.List[0].Type)
			if strings.Contains(firstParamType, "testing.M") {
				continue
			}
		}
		if strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "_" {
			testFuncs = append(testFuncs, fn)
		} else if !strings.HasPrefix(fn.Name.Name, "Benchmark") && !strings.HasPrefix(fn.Name.Name, "Example") {
			helpers[fn.Name.Name] = fn
		}
	}

	for _, fn := range testFuncs {
		localConsts := make(map[string]constResult)
		for k, v := range fileConsts {
			localConsts[k] = v
		}
		unsignedVars := make(map[string]bool)
		closures := make(map[string]*ast.FuncLit)
		calledClosures := make(map[string]bool)

		// Check defer recover
		for _, stmt := range fn.Body.List {
			if defStmt, ok := stmt.(*ast.DeferStmt); ok {
				ast.Inspect(defStmt.Call, func(dn ast.Node) bool {
					if id, ok := dn.(*ast.Ident); ok && id.Name == "recover" {
						hasRecoverAssertion := false
						if lit, ok := defStmt.Call.Fun.(*ast.FuncLit); ok && lit.Body != nil {
							ast.Inspect(lit.Body, func(ln ast.Node) bool {
								if call, ok := ln.(*ast.CallExpr); ok {
									if isRealAssertionCall(call) {
										hasRecoverAssertion = true
									}
									if pId, ok := call.Fun.(*ast.Ident); ok && pId.Name == "panic" {
										hasRecoverAssertion = true
									}
								}
								return true
							})
						}
						hasFuncAssertion := false
						ast.Inspect(fn.Body, func(bn ast.Node) bool {
							if call, ok := bn.(*ast.CallExpr); ok && isRealAssertionCall(call) {
								hasFuncAssertion = true
							}
							return true
						})
						if !hasRecoverAssertion && !hasFuncAssertion {
							violations = append(violations, fmt.Sprintf("test integrity violation: defer recover() panic swallowing detected in test %s", fn.Name.Name))
						}
					}
					return true
				})
			}
		}

		// Track declarations and variables
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if declStmt, ok := n.(*ast.DeclStmt); ok {
				if gen, ok := declStmt.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
					for _, spec := range gen.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							typeStr := formatExpr(vs.Type)
							if strings.HasPrefix(typeStr, "uint") || typeStr == "uintptr" {
								for _, name := range vs.Names {
									unsignedVars[name.Name] = true
								}
							}
						}
					}
				}
			}
			if gen, ok := n.(*ast.GenDecl); ok && gen.Tok == token.CONST {
				for _, spec := range gen.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for i, name := range vs.Names {
							if i < len(vs.Values) {
								if res, ok := evalConstExpr(vs.Values[i], localConsts, unsignedVars); ok {
									localConsts[name.Name] = res
								}
							}
						}
					}
				}
			}
			return true
		})

		// Track sync join calls in test function
		outerJoins := &syncJoinInfo{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Go" {
					outerJoins.errgroupGoCount++
				}
				if sel.Sel.Name == "Wait" {
					outerJoins.hasWaitCall = true
				}
			}
			if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
				outerJoins.hasChanRecv = true
			}
			return true
		})

		foundAssertion, _, blockViolations := checkASTBlock(fn.Body.List, true, localConsts, unsignedVars, helpers, closures, calledClosures, outerJoins)
		violations = append(violations, blockViolations...)

		if outerJoins.errgroupGoCount > 0 && !outerJoins.hasWaitCall {
			violations = append(violations, fmt.Sprintf("test integrity violation: errgroup.Go without Wait() uncoordinated execution detected in test %s", fn.Name.Name))
		}

		if !foundAssertion {
			if outerJoins.hasWaitCall || outerJoins.hasChanRecv || outerJoins.goroutineWithAssertionCount > 0 || outerJoins.goroutineCount > 0 {
				if outerJoins.goroutineWithAssertionCount > 0 && !outerJoins.hasWaitCall && !outerJoins.hasChanRecv {
					violations = append(violations, fmt.Sprintf("test integrity violation: test %s has assertions only inside unjoined goroutine", fn.Name.Name))
				} else {
					foundAssertion = true
				}
			} else if fn.Name.Name != "_" {
				violations = append(violations, fmt.Sprintf("test integrity violation: test %s has no reachable assertions", fn.Name.Name))
			} else {
				violations = append(violations, "test integrity violation: test snippet has no reachable assertions")
			}
		}
	}

	return deduplicateStrings(violations)
}

// CountTestsInWorkspace counts all Test* functions in all Go test files within workspaceDir.
func CountTestsInWorkspace(workspaceDir string) int {
	if workspaceDir == "" {
		return 0
	}
	count := 0
	fset := token.NewFileSet()
	_ = filepath.Walk(workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == ".gradle" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			if node, err := parser.ParseFile(fset, path, nil, 0); err == nil {
				for _, decl := range node.Decls {
					if fn, ok := decl.(*ast.FuncDecl); ok {
						if strings.HasPrefix(fn.Name.Name, "Test") {
							count++
						}
					}
				}
			}
		}
		return nil
	})
	return count
}

// ExtractCoverage parses statement coverage percentage from test output.
func ExtractCoverage(output string) float64 {
	re := regexp.MustCompile(`coverage:\s*([0-9]+\.?[0-9]*)%`)
	m := re.FindStringSubmatch(output)
	if len(m) > 1 {
		var val float64
		if _, err := fmt.Sscanf(m[1], "%f", &val); err == nil {
			return val / 100.0
		}
	}
	return 0.0
}

