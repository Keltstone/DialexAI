package scan

import (
	"errors"
	"fmt"
	"strings"
)

// ScanPython extracts public API symbols and canonical signatures from Python sources.
func ScanPython(src []byte) (map[string]string, error) {
	s, err := Lex(Python, src)
	if err != nil {
		return nil, err
	}
	return ScanPythonSource(s)
}

// LogicalLine represents a logical line in Python (after joining physical lines via brackets/backslash).
type LogicalLine struct {
	Indent int
	Start  int
	End    int
	Tokens []Token
}

// ScanPythonSource extracts symbols from a lexed Python Source.
func ScanPythonSource(s *Source) (res map[string]string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during Python scanning: %v", r)
		}
	}()

	lines, err := buildLogicalLines(s)
	if err != nil {
		return nil, err
	}

	c := &Collector{}
	w := &pythonWalker{s: s, col: c, lines: lines}
	err = w.walk()
	if err != nil {
		return nil, err
	}

	return c.Finalize(), nil
}

type pythonWalker struct {
	s     *Source
	col   *Collector
	lines []LogicalLine
}

func buildLogicalLines(s *Source) ([]LogicalLine, error) {
	var lines []LogicalLine
	n := len(s.B)
	i := 0

	for i < n {
		// Calculate indent of this line
		lineStart := i
		indent := 0
		hasCode := false

		for i < n && s.B[i] != '\n' {
			if s.K[i] == Code {
				c := s.B[i]
				if c == ' ' && !hasCode {
					indent++
				} else if c == '\t' && !hasCode {
					indent = (indent/8 + 1) * 8
				} else {
					hasCode = true
				}
			} else if s.K[i] == Str {
				hasCode = true
			}
			i++
		}

		if i < n && s.B[i] == '\n' {
			i++
		}

		if !hasCode {
			// Blank or comment-only line
			continue
		}

		// Check if logical line continues across newlines due to unclosed brackets or trailing backslash
		logicalStart := lineStart
		bracketDepth := 0
		curr := logicalStart

		for curr < n {
			if s.K[curr] == Code {
				c := s.B[curr]
				if c == '(' || c == '[' || c == '{' {
					bracketDepth++
				} else if c == ')' || c == ']' || c == '}' {
					if bracketDepth > 0 {
						bracketDepth--
					}
				} else if c == '\n' {
					if bracketDepth == 0 {
						// Check if preceded by backslash \
						prev := s.PrevCode(curr)
						if prev < 0 || s.B[prev] != '\\' {
							// End of logical line
							break
						}
					}
				}
			}
			curr++
		}

		logicalEnd := curr
		i = curr
		if i < n && s.B[i] == '\n' {
			i++
		}

		toks := Tokenize(s, logicalStart, logicalEnd)
		// Strip backslash tokens used for line continuation
		var cleanToks []Token
		for _, t := range toks {
			if t.Text != "\\" {
				cleanToks = append(cleanToks, t)
			}
		}

		if len(cleanToks) > 0 {
			lines = append(lines, LogicalLine{
				Indent: indent,
				Start:  logicalStart,
				End:    logicalEnd,
				Tokens: cleanToks,
			})
		}
	}

	return lines, nil
}

func (w *pythonWalker) walk() error {
	lines := w.lines
	if len(lines) == 0 {
		return nil
	}

	// 1. Scan for module-level __all__
	allMap, hasAll, err := w.extractAllList()
	if err != nil {
		return err
	}

	// 2. Parse statements with block indentation stack
	type scopeInfo struct {
		indent int
		prefix string
	}

	indentStack := []scopeInfo{{indent: 0, prefix: ""}}
	var pendingDecorators []string

	lineIdx := 0
	for lineIdx < len(lines) {
		line := lines[lineIdx]
		toks := line.Tokens

		// Handle block dedent
		for len(indentStack) > 1 && line.Indent < indentStack[len(indentStack)-1].indent {
			indentStack = indentStack[:len(indentStack)-1]
		}

		if line.Indent > indentStack[len(indentStack)-1].indent {
			// Inconsistent indent without a block statement or inside a skipped body
			lineIdx++
			continue
		}

		currScope := indentStack[len(indentStack)-1]

		// Check for decorator: starts with '@'
		if toks[0].Text == "@" {
			pendingDecorators = append(pendingDecorators, Join(toks))
			lineIdx++
			continue
		}

		// Skip import statements
		if toks[0].Text == "import" || toks[0].Text == "from" {
			pendingDecorators = nil
			lineIdx++
			continue
		}

		// Skip if __name__ == "__main__":
		if toks[0].Text == "if" && strings.Contains(Join(toks), `__name__ == "__main__"`) {
			// Skip this entire block
			targetIndent := line.Indent
			lineIdx++
			for lineIdx < len(lines) && lines[lineIdx].Indent > targetIndent {
				lineIdx++
			}
			pendingDecorators = nil
			continue
		}

		// Code under `if TYPE_CHECKING:` is scanned normally: just unwrap / enter the block
		if toks[0].Text == "if" && strings.Contains(Join(toks), "TYPE_CHECKING") {
			pendingDecorators = nil
			lineIdx++
			continue
		}

		// Function definition: [async] def name(params) -> ret:
		isAsync := false
		defIdx := 0
		if toks[0].Text == "async" && len(toks) > 1 && toks[1].Text == "def" {
			isAsync = true
			defIdx = 1
		}

		if toks[defIdx].Text == "def" {
			fnName := ""
			if defIdx+1 < len(toks) {
				fnName = toks[defIdx+1].Text
			}

			isPublic := isPythonPublicName(fnName)
			if currScope.prefix == "" && hasAll {
				if !allMap[fnName] {
					isPublic = false
				}
			}

			key := fnName
			if currScope.prefix != "" {
				key = currScope.prefix + "." + fnName
			}

			// Clean header up to depth-0 ':' after param list
			headerSig := w.buildDefHeader(toks, pendingDecorators)
			paramTypes := w.extractPythonParamTypes(toks)

			if isPublic {
				w.col.Add(key, headerSig, paramTypes...)
			}

			// Function bodies are skipped! Skip all lines more indented than def
			targetIndent := line.Indent
			lineIdx++
			for lineIdx < len(lines) && lines[lineIdx].Indent > targetIndent {
				lineIdx++
			}
			pendingDecorators = nil
			_ = isAsync
			continue
		}

		// Class definition: class Name(bases):
		if toks[0].Text == "class" {
			className := ""
			if len(toks) > 1 {
				className = toks[1].Text
			}

			isPublic := isPythonPublicName(className)
			if currScope.prefix == "" && hasAll {
				if !allMap[className] {
					isPublic = false
				}
			}

			fullKey := className
			if currScope.prefix != "" {
				fullKey = currScope.prefix + "." + className
			}

			headerSig := w.buildClassHeader(toks, pendingDecorators)
			if isPublic {
				w.col.Add(fullKey, headerSig)
			}

			// Enter class body
			targetIndent := line.Indent
			// Find body indent
			bodyIndent := -1
			if lineIdx+1 < len(lines) && lines[lineIdx+1].Indent > targetIndent {
				bodyIndent = lines[lineIdx+1].Indent
			}
			if bodyIndent > 0 && isPublic {
				indentStack = append(indentStack, scopeInfo{
					indent: bodyIndent,
					prefix: fullKey,
				})
			} else if bodyIndent > 0 && !isPublic {
				// Non-public class hides its members: skip entire class body
				lineIdx++
				for lineIdx < len(lines) && lines[lineIdx].Indent > targetIndent {
					lineIdx++
				}
				pendingDecorators = nil
				continue
			}

			pendingDecorators = nil
			lineIdx++
			continue
		}

		// Type alias (Python 3.12 `type X = ...`)
		if toks[0].Text == "type" && len(toks) >= 4 && toks[2].Text == "=" {
			aliasName := toks[1].Text
			isPublic := isPythonPublicName(aliasName)
			if currScope.prefix == "" && hasAll {
				if !allMap[aliasName] {
					isPublic = false
				}
			}
			key := aliasName
			if currScope.prefix != "" {
				key = currScope.prefix + "." + aliasName
			}
			if isPublic {
				w.col.Add(key, Join(toks))
			}
			pendingDecorators = nil
			lineIdx++
			continue
		}

		// Assignments: NAME: T = v or NAME = v
		if len(toks) >= 2 && toks[0].Type == TokIdent {
			name := toks[0].Text
			isPublic := isPythonPublicName(name)
			if currScope.prefix == "" && hasAll {
				if !allMap[name] {
					isPublic = false
				}
			}

			key := name
			if currScope.prefix != "" {
				key = currScope.prefix + "." + name
			}

			hasColon := (toks[1].Text == ":")
			eqIdx := -1
			for idx, t := range toks {
				if t.Text == "=" {
					eqIdx = idx
					break
				}
			}

			if hasColon || eqIdx == 1 {
				if isPublic {
					sig := w.buildPyAssignmentSig(name, toks, hasColon, eqIdx)
					w.col.Add(key, sig)
				}
				pendingDecorators = nil
				lineIdx++
				continue
			}
		}

		pendingDecorators = nil
		lineIdx++
	}

	return nil
}

func (w *pythonWalker) buildPyAssignmentSig(name string, toks []Token, hasColon bool, eqIdx int) string {
	isUpper := isUpperSnake(name)

	var sig string
	if hasColon {
		// Type annotation exists: up to '='
		colonEnd := len(toks)
		if eqIdx >= 0 {
			colonEnd = eqIdx
		}
		sig = Join(toks[:colonEnd])
	} else {
		sig = name
	}

	if isUpper && eqIdx >= 0 {
		// If literal (<= 4 KB), append = <canonical literal>
		valToks := toks[eqIdx+1:]
		if isPyLiteral(valToks) {
			valStr := Join(valToks)
			if len(valStr) <= 4*1024 {
				sig = sig + " = " + valStr
			}
		}
	}

	return sig
}

func isUpperSnake(s string) bool {
	if len(s) == 0 {
		return false
	}
	hasLetter := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			return false
		}
		if c >= 'A' && c <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
}

func isPyLiteral(toks []Token) bool {
	if len(toks) == 0 {
		return false
	}
	if len(toks) == 1 {
		t := toks[0]
		if t.Type == TokStr {
			return true
		}
		if t.Type == TokIdent {
			return t.Text == "True" || t.Text == "False" || t.Text == "None" || isNumeric(t.Text)
		}
	}
	// List or tuple of literals: [1, 2] or ("a", "b")
	if (toks[0].Text == "[" && toks[len(toks)-1].Text == "]") ||
		(toks[0].Text == "(" && toks[len(toks)-1].Text == ")") {
		return true
	}
	return false
}

func isPythonPublicName(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, "_") {
		// Dunder names are public: starts with __ and ends with __
		if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") && len(name) >= 4 {
			return true
		}
		return false
	}
	return true
}

func (w *pythonWalker) extractAllList() (map[string]bool, bool, error) {
	allMap := make(map[string]bool)
	hasAll := false

	for _, line := range w.lines {
		if line.Indent != 0 {
			continue
		}
		toks := line.Tokens
		if len(toks) >= 3 && toks[0].Text == "__all__" {
			op := toks[1].Text
			if op != "=" && op != "+=" {
				continue
			}
			hasAll = true
			// Must be list or tuple of string literals only
			valToks := toks[2:]
			if len(valToks) < 2 {
				return nil, false, errors.New("symbols: dynamic __all__")
			}
			openBracket := valToks[0].Text
			closeBracket := valToks[len(valToks)-1].Text
			if (openBracket != "[" || closeBracket != "]") && (openBracket != "(" || closeBracket != ")") {
				return nil, false, errors.New("symbols: dynamic __all__")
			}

			// Validate contents: only TokStr or comma
			for _, t := range valToks[1 : len(valToks)-1] {
				if t.Text == "," {
					continue
				}
				if t.Type != TokStr {
					return nil, false, errors.New("symbols: dynamic __all__")
				}
				strVal := strings.Trim(t.Text, `"'`)
				allMap[strVal] = true
			}
		}
	}

	return allMap, hasAll, nil
}

func (w *pythonWalker) buildDefHeader(toks []Token, decorators []string) string {
	// Header ends at depth-0 ':' after param list
	colonIdx := -1
	depth := 0
	for i, t := range toks {
		if t.Text == "(" || t.Text == "[" || t.Text == "{" {
			depth++
		} else if t.Text == ")" || t.Text == "]" || t.Text == "}" {
			depth--
		}
		if t.Text == ":" && depth == 0 {
			colonIdx = i
			break
		}
	}

	headerToks := toks
	if colonIdx >= 0 {
		headerToks = toks[:colonIdx+1]
	}

	sig := Join(headerToks)
	if len(decorators) > 0 {
		sig = strings.Join(decorators, " ") + " " + sig
	}
	return sig
}

func (w *pythonWalker) buildClassHeader(toks []Token, decorators []string) string {
	colonIdx := -1
	depth := 0
	for i, t := range toks {
		if t.Text == "(" || t.Text == "[" || t.Text == "{" {
			depth++
		} else if t.Text == ")" || t.Text == "]" || t.Text == "}" {
			depth--
		}
		if t.Text == ":" && depth == 0 {
			colonIdx = i
			break
		}
	}

	headerToks := toks
	if colonIdx >= 0 {
		headerToks = toks[:colonIdx+1]
	}

	sig := Join(headerToks)
	if len(decorators) > 0 {
		sig = strings.Join(decorators, " ") + " " + sig
	}
	return sig
}

func (w *pythonWalker) extractPythonParamTypes(toks []Token) []string {
	// Find '(' and matching ')'
	openIdx := -1
	closeIdx := -1
	depth := 0
	for i, t := range toks {
		if t.Text == "(" {
			if depth == 0 {
				openIdx = i
			}
			depth++
		} else if t.Text == ")" {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}

	if openIdx < 0 || closeIdx <= openIdx+1 {
		return nil
	}

	// Split parameters by top-level ','
	var params [][]Token
	var cur []Token
	pDepth := 0
	for _, t := range toks[openIdx+1 : closeIdx] {
		if t.Text == "(" || t.Text == "[" || t.Text == "{" {
			pDepth++
		} else if t.Text == ")" || t.Text == "]" || t.Text == "}" {
			pDepth--
		}
		if t.Text == "," && pDepth == 0 {
			if len(cur) > 0 {
				params = append(params, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		params = append(params, cur)
	}

	var paramTypes []string
	for _, p := range params {
		// Skip 'self' or 'cls' without annotation
		if len(p) == 1 && (p[0].Text == "self" || p[0].Text == "cls") {
			continue
		}
		// Look for ':'
		colonIdx := -1
		for idx, t := range p {
			if t.Text == ":" {
				colonIdx = idx
				break
			}
		}
		if colonIdx >= 0 {
			// Type annotation is after ':' up to '=' or end
			typeToks := p[colonIdx+1:]
			eqIdx := -1
			for idx, t := range typeToks {
				if t.Text == "=" {
					eqIdx = idx
					break
				}
			}
			if eqIdx >= 0 {
				typeToks = typeToks[:eqIdx]
			}
			paramTypes = append(paramTypes, Join(typeToks))
		} else {
			paramTypes = append(paramTypes, "_")
		}
	}

	return paramTypes
}
