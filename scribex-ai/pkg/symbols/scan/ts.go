package scan

import (
	"fmt"
	"strings"
)

// ScanTS extracts public API symbols and canonical signatures from TypeScript and JavaScript sources.
func ScanTS(src []byte) (map[string]string, error) {
	s, err := Lex(TS, src)
	if err != nil {
		return nil, err
	}
	return ScanTSSource(s)
}

// ScanTSDetailed extracts detailed public API symbols from TypeScript and JavaScript sources.
func ScanTSDetailed(src []byte) ([]Symbol, error) {
	s, err := Lex(TS, src)
	if err != nil {
		return nil, err
	}
	return ScanTSSourceDetailed(s)
}

// ScanTSSource extracts symbols from a lexed TS Source.
func ScanTSSource(s *Source) (res map[string]string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during TS scanning: %v", r)
		}
	}()

	c := &Collector{}
	w := &tsWalker{s: s, col: c}
	err := w.scanScope(0, len(s.B), "")
	if err != nil {
		return nil, err
	}
	return c.Finalize(), nil
}

// ScanTSSourceDetailed extracts detailed symbols from a lexed TS Source.
func ScanTSSourceDetailed(s *Source) (res []Symbol, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during TS scanning: %v", r)
		}
	}()

	c := &Collector{}
	w := &tsWalker{s: s, col: c}
	err := w.scanScope(0, len(s.B), "")
	if err != nil {
		return nil, err
	}
	return c.FinalizeDetailed(s, TS), nil
}

type tsWalker struct {
	s   *Source
	col *Collector
}

func (w *tsWalker) scanScope(scopeStart, scopeEnd int, prefix string) error {
	s := w.s
	n := scopeEnd
	i := scopeStart

	for i < n {
		i = s.NextCode(i)
		if i >= n {
			break
		}

		// CommonJS top-level checks if prefix is empty
		if prefix == "" {
			handled, nextI, err := w.checkCommonJS(i, n)
			if err != nil {
				return err
			}
			if handled {
				i = nextI
				continue
			}
		}

		word, wordEnd := w.readWord(i, n)
		if word != "export" {
			// Skip non-export statement
			i = w.skipStatement(i, n)
			continue
		}

		stmtStart := i
		i = wordEnd

		// What follows export?
		i = s.NextCode(i)
		if i >= n {
			break
		}

		// Check for `export default`
		nextWord, nextWordEnd := w.readWord(i, n)
		if nextWord == "default" {
			i = s.NextCode(nextWordEnd)
			// Can be `export default function ...`, `export default class ...`, or `export default <expr>`
			defWord, _ := w.readWord(i, n)
			if defWord == "function" || defWord == "class" || defWord == "abstract" || defWord == "async" {
				// Function or class declaration
				headerEnd, err := FindBraceHeaderEnd(s, stmtStart)
				if err != nil {
					return err
				}
				sigToks := Tokenize(s, stmtStart, headerEnd)
				w.col.At(stmtStart)
				w.col.Add("default", Join(sigToks))
				if headerEnd < n && s.CodeAt(headerEnd) == '{' {
					cl, err := s.MatchClose(headerEnd)
					if err != nil {
						return err
					}
					i = cl + 1
				} else {
					i = headerEnd + 1
				}
				continue
			}

			// Expression export default <expr>
			exprEnd := w.findStatementEnd(i, n)
			toks := Tokenize(s, i, exprEnd)
			if len(toks) > 200 {
				toks = toks[:200]
			}
			w.col.At(stmtStart)
			w.col.Add("default", "default "+Join(toks))
			i = exprEnd
			continue
		}

		// Check for re-exports: `export { a, b as c } [from "m"]` or `export * from "m"`
		if s.B[i] == '{' {
			cl, err := s.MatchClose(i)
			if err != nil {
				return err
			}
			stmtEnd := w.findStatementEnd(cl+1, n)
			clauseToks := Tokenize(s, stmtStart, stmtEnd)
			reexportClause := Join(clauseToks)

			// Extract exported symbol names from inside { ... }
			insideToks := Tokenize(s, i+1, cl)
			names := w.extractReexportedNames(insideToks)
			for _, name := range names {
				w.col.At(stmtStart)
				w.col.Add(name, reexportClause)
			}
			i = stmtEnd
			continue
		}

		if s.B[i] == '*' {
			// `export * from "m"` or `export * as ns from "m"`
			stmtEnd := w.findStatementEnd(i+1, n)
			toks := Tokenize(s, stmtStart, stmtEnd)
			asIdx := -1
			fromIdx := -1
			for idx, t := range toks {
				if t.Text == "as" {
					asIdx = idx
				} else if t.Text == "from" {
					fromIdx = idx
				}
			}
			if asIdx >= 0 && asIdx+1 < len(toks) {
				nsName := toks[asIdx+1].Text
				w.col.At(stmtStart)
				w.col.Add(nsName, Join(toks))
			} else if fromIdx >= 0 && fromIdx+1 < len(toks) {
				modName := strings.Trim(toks[fromIdx+1].Text, `"'`)
				w.col.At(stmtStart)
				w.col.Add("*:"+modName, Join(toks))
			}
			i = stmtEnd
			continue
		}

		// Modifiers: [declare] [abstract] [async]
		for i < n {
			wrd, wEnd := w.readWord(i, n)
			if wrd == "declare" || wrd == "abstract" || wrd == "async" {
				i = s.NextCode(wEnd)
				continue
			}
			break
		}

		declKw, declKwEnd := w.readWord(i, n)
		switch declKw {
		case "function":
			// export function[*] name<T>(params): R
			fnStart := declKwEnd
			if fnStart < n && s.B[fnStart] == '*' {
				fnStart++
			}
			headerEnd, err := FindBraceHeaderEnd(s, stmtStart)
			if err != nil {
				return err
			}
			parenIdx := -1
			for j := fnStart; j < headerEnd; j++ {
				if s.K[j] == Code && s.B[j] == '(' {
					parenIdx = j
					break
				}
			}
			name := ""
			if parenIdx >= 0 {
				name = w.extractNameBeforeParen(fnStart, parenIdx)
			} else {
				name = w.extractTypeName(fnStart, headerEnd)
			}
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}

			paramStart, paramEnd := w.findParenRange(fnStart, headerEnd)
			paramTypes := ExtractParamTypes(s, paramStart, paramEnd)

			sigToks := Tokenize(s, stmtStart, headerEnd)
			w.col.At(stmtStart)
			w.col.Add(key, Join(sigToks), paramTypes...)

			if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "class":
			// export [abstract] class Name ... { ... }
			headerEnd, err := FindBraceHeaderEnd(s, stmtStart)
			if err != nil {
				return err
			}
			name := w.extractTypeName(declKwEnd, headerEnd)
			fullKey := name
			if prefix != "" {
				fullKey = prefix + "." + name
			}

			sigToks := Tokenize(s, stmtStart, headerEnd)
			w.col.At(stmtStart)
			w.col.Add(fullKey, Join(sigToks))

			if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				err = w.scanClassMembers(headerEnd+1, cl, fullKey)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "interface":
			// export interface Name<T> { ... }
			headerEnd, err := FindBraceHeaderEnd(s, stmtStart)
			if err != nil {
				return err
			}
			name := w.extractTypeName(declKwEnd, headerEnd)
			fullKey := name
			if prefix != "" {
				fullKey = prefix + "." + name
			}

			sigToks := Tokenize(s, stmtStart, headerEnd)
			w.col.At(stmtStart)
			w.col.Add(fullKey, Join(sigToks))

			if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				err = w.scanInterfaceMembers(headerEnd+1, cl, fullKey)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "type":
			// export type X<T> = ...
			stmtEnd := w.findStatementEnd(declKwEnd, n)
			name := w.extractTypeName(declKwEnd, stmtEnd)
			fullKey := name
			if prefix != "" {
				fullKey = prefix + "." + name
			}

			sigToks := Tokenize(s, stmtStart, stmtEnd)
			w.col.At(stmtStart)
			w.col.Add(fullKey, Join(sigToks))
			i = stmtEnd
			continue

		case "enum":
			// export [const] enum Name { ... }
			headerEnd, err := FindBraceHeaderEnd(s, stmtStart)
			if err != nil {
				return err
			}
			name := w.extractTypeName(declKwEnd, headerEnd)
			fullKey := name
			if prefix != "" {
				fullKey = prefix + "." + name
			}

			sigToks := Tokenize(s, stmtStart, headerEnd)
			w.col.At(stmtStart)
			w.col.Add(fullKey, Join(sigToks))

			if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				err = w.scanEnumMembers(headerEnd+1, cl, fullKey)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "const", "let", "var":
			// export const|let|var name[: T] [= value]
			stmtEnd := w.findStatementEnd(declKwEnd, n)
			err := w.scanVariableDeclarators(stmtStart, declKw, declKwEnd, stmtEnd, prefix)
			if err != nil {
				return err
			}
			i = stmtEnd
			continue

		case "namespace", "module":
			// export namespace X { ... }
			headerEnd, err := FindBraceHeaderEnd(s, stmtStart)
			if err != nil {
				return err
			}
			name := w.extractTypeName(declKwEnd, headerEnd)
			fullKey := name
			if prefix != "" {
				fullKey = prefix + "." + name
			}

			if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				err = w.scanScope(headerEnd+1, cl, fullKey)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue
		}

		i = w.skipStatement(i, n)
	}

	return nil
}

func (w *tsWalker) scanClassMembers(start, end int, classPrefix string) error {
	s := w.s
	n := end
	i := start

	for i < n {
		i = s.NextCode(i)
		if i >= n {
			break
		}

		memberStart := i
		// Check for private / protected / # private field
		isPublic := true
		isStatic := false
		isReadonly := false

		// Read member modifiers
		for i < n {
			wrd, wEnd := w.readWord(i, n)
			if wrd == "private" || wrd == "protected" {
				isPublic = false
				i = s.NextCode(wEnd)
				continue
			}
			if wrd == "static" {
				isStatic = true
				i = s.NextCode(wEnd)
				continue
			}
			if wrd == "readonly" {
				isReadonly = true
				i = s.NextCode(wEnd)
				continue
			}
			if wrd == "public" || wrd == "abstract" || wrd == "override" || wrd == "async" || wrd == "get" || wrd == "set" || wrd == "declare" {
				i = s.NextCode(wEnd)
				continue
			}
			break
		}

		if i < n && s.B[i] == '#' {
			isPublic = false // private identifier #field
		}

		// Find member header end
		headerEnd, err := FindBraceHeaderEnd(s, memberStart)
		if err != nil {
			return err
		}

		// Check index signature [key: string]: T
		if i < n && s.B[i] == '[' {
			if isPublic {
				sigToks := Tokenize(s, memberStart, headerEnd)
				w.col.At(memberStart)
				w.col.Add(classPrefix+".[]", Join(sigToks))
			}
			i = headerEnd + 1
			continue
		}

		parenIdx := -1
		for j := i; j < headerEnd; j++ {
			if s.K[j] == Code && s.B[j] == '(' {
				parenIdx = j
				break
			}
		}

		if parenIdx >= 0 {
			// Method or constructor
			name := w.extractNameBeforeParen(i, parenIdx)
			var key string
			isCtor := (name == "constructor")
			if isCtor {
				key = classPrefix + ".constructor"
			} else {
				key = classPrefix + "." + name
			}

			paramStart, paramEnd := w.findParenRange(parenIdx, headerEnd)
			paramTypes := ExtractParamTypes(s, paramStart, paramEnd)

			if isPublic {
				sigToks := Tokenize(s, memberStart, headerEnd)
				w.col.At(memberStart)
				w.col.Add(key, Join(sigToks), paramTypes...)
			}

			// In constructor, check for parameter properties: constructor(public x: number)
			if isCtor && isPublic {
				w.extractParameterProperties(paramStart, paramEnd, classPrefix)
			}

			if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue
		}

		// Property: name?: T [= literal]
		propName, eqIdx := w.extractFieldNameAndEqual(i, headerEnd)
		if propName != "" && isPublic {
			var sigToks []Token
			if isStatic && isReadonly && eqIdx >= 0 && w.isLiteralInit(eqIdx+1, headerEnd) {
				sigToks = Tokenize(s, memberStart, headerEnd)
			} else {
				cutEnd := headerEnd
				if eqIdx >= 0 {
					cutEnd = eqIdx
				}
				sigToks = Tokenize(s, memberStart, cutEnd)
			}
			w.col.At(memberStart)
			w.col.Add(classPrefix+"."+propName, Join(sigToks))
		}

		i = headerEnd + 1
	}

	return nil
}

func (w *tsWalker) scanInterfaceMembers(start, end int, ifacePrefix string) error {
	s := w.s
	n := end
	i := start

	for i < n {
		i = s.NextCode(i)
		if i >= n {
			break
		}

		memberStart := i
		// Call signature () or Construct signature new()
		if s.B[i] == '(' {
			cl, err := s.MatchClose(i)
			if err != nil {
				return err
			}
			stmtEnd := w.findStatementEnd(cl+1, n)
			sigToks := Tokenize(s, memberStart, stmtEnd)
			w.col.At(memberStart)
			w.col.Add(ifacePrefix+".()", Join(sigToks))
			i = stmtEnd
			continue
		}
		word, wordEnd := w.readWord(i, n)
		if word == "new" {
			stmtEnd := w.findStatementEnd(wordEnd, n)
			sigToks := Tokenize(s, memberStart, stmtEnd)
			w.col.At(memberStart)
			w.col.Add(ifacePrefix+".new", Join(sigToks))
			i = stmtEnd
			continue
		}

		// Index signature [key: string]: T
		if s.B[i] == '[' {
			stmtEnd := w.findStatementEnd(i, n)
			sigToks := Tokenize(s, memberStart, stmtEnd)
			w.col.At(memberStart)
			w.col.Add(ifacePrefix+".[]", Join(sigToks))
			i = stmtEnd
			continue
		}

		stmtEnd := w.findStatementEnd(i, n)
		name := word
		if name != "" {
			sigToks := Tokenize(s, memberStart, stmtEnd)
			w.col.At(memberStart)
			w.col.Add(ifacePrefix+"."+name, Join(sigToks))
		}
		i = stmtEnd
	}

	return nil
}

func (w *tsWalker) scanEnumMembers(start, end int, enumPrefix string) error {
	s := w.s
	n := end
	i := start

	for i < n {
		i = s.NextCode(i)
		if i >= n {
			break
		}
		if s.B[i] == '}' {
			break
		}

		memberStart := i
		name, nameEnd := w.readWord(i, n)
		if name == "" {
			i++
			continue
		}

		// Member ends at ',' or '}' or end
		mEnd := nameEnd
		for mEnd < n {
			if s.K[mEnd] == Code && (s.B[mEnd] == ',' || s.B[mEnd] == '}') {
				break
			}
			mEnd++
		}

		sigToks := Tokenize(s, memberStart, mEnd)
		w.col.At(memberStart)
		w.col.Add(enumPrefix+"."+name, Join(sigToks))

		i = mEnd
		if i < n && s.B[i] == ',' {
			i++
		}
	}
	return nil
}

func (w *tsWalker) scanVariableDeclarators(exportStart int, declKw string, start, end int, prefix string) error {
	s := w.s
	// Multiple declarators: a = 1, b = 2
	toks := Tokenize(s, start, end)
	// Split by top-level ','
	var declarators [][]Token
	var cur []Token
	depth := 0
	for _, t := range toks {
		if t.Text == "(" || t.Text == "[" || t.Text == "{" {
			depth++
		} else if t.Text == ")" || t.Text == "]" || t.Text == "}" {
			depth--
		}
		if t.Text == "," && depth == 0 {
			if len(cur) > 0 {
				declarators = append(declarators, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		declarators = append(declarators, cur)
	}

	for _, dtoks := range declarators {
		if len(dtoks) == 0 {
			continue
		}
		if dtoks[len(dtoks)-1].Text == ";" {
			dtoks = dtoks[:len(dtoks)-1]
		}
		if len(dtoks) == 0 {
			continue
		}
		name := dtoks[0].Text
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		var sigToks []Token
		sigToks = append(sigToks, Token{TokIdent, "export"}, Token{TokIdent, declKw})

		eqIdx := -1
		for idx, t := range dtoks {
			if t.Text == "=" {
				eqIdx = idx
				break
			}
		}

		if declKw == "const" && eqIdx >= 0 && w.isTokenListLiteral(dtoks[eqIdx+1:]) {
			// const with literal initializer: include literal
			sigToks = append(sigToks, dtoks...)
		} else {
			// Otherwise annotation only (up to '=')
			cut := len(dtoks)
			if eqIdx >= 0 {
				cut = eqIdx
			}
			sigToks = append(sigToks, dtoks[:cut]...)
		}

		w.col.At(start)
		w.col.Add(key, Join(sigToks))
	}

	return nil
}

func (w *tsWalker) checkCommonJS(start, end int) (bool, int, error) {
	s := w.s
	// CommonJS forms: module.exports = ... , exports.name = ... , module.exports.name = ...
	toks := Tokenize(s, start, w.findStatementEnd(start, end))
	if len(toks) < 3 {
		return false, start, nil
	}

	// exports.name = ...
	if toks[0].Text == "exports" && toks[1].Text == "." && len(toks) >= 4 && toks[3].Text == "=" {
		name := toks[2].Text
		stmtEnd := w.findStatementEnd(start, end)
		sig := w.buildCJSMethodSig(toks)
		w.col.At(start)
		w.col.Add(name, sig)
		return true, stmtEnd, nil
	}

	// module.exports.name = ...
	if len(toks) >= 6 && toks[0].Text == "module" && toks[1].Text == "." && toks[2].Text == "exports" && toks[3].Text == "." && toks[5].Text == "=" {
		name := toks[4].Text
		stmtEnd := w.findStatementEnd(start, end)
		sig := w.buildCJSMethodSig(toks)
		w.col.At(start)
		w.col.Add(name, sig)
		return true, stmtEnd, nil
	}

	// module.exports = { a, b } or module.exports = function ...
	if len(toks) >= 4 && toks[0].Text == "module" && toks[1].Text == "." && toks[2].Text == "exports" && toks[3].Text == "=" {
		stmtEnd := w.findStatementEnd(start, end)
		if len(toks) >= 5 && toks[4].Text == "{" {
			// object export { a, b }
			inside := toks[5:]
			for _, t := range inside {
				if t.Type == TokIdent && t.Text != "}" {
					w.col.At(start)
					w.col.Add(t.Text, "exports."+t.Text)
				}
			}
		} else {
			w.col.At(start)
			w.col.Add("module.exports", Join(toks))
		}
		return true, stmtEnd, nil
	}

	return false, start, nil
}

func (w *tsWalker) buildCJSMethodSig(toks []Token) string {
	// If right hand side has function(params) -> exports.name function(params)
	fnIdx := -1
	for i, t := range toks {
		if t.Text == "function" {
			fnIdx = i
			break
		}
	}
	if fnIdx >= 0 {
		return Join(toks[:3]) + " " + Join(toks[fnIdx:])
	}
	return Join(toks[:3])
}

func (w *tsWalker) extractParameterProperties(paramStart, paramEnd int, classPrefix string) {
	s := w.s
	paramToks := Tokenize(s, paramStart, paramEnd)
	// Look for tokens like public x: number
	for i := 0; i < len(paramToks); i++ {
		t := paramToks[i]
		if t.Text == "public" || t.Text == "readonly" {
			// Extract name and type
			if i+1 < len(paramToks) && paramToks[i+1].Type == TokIdent {
				propName := paramToks[i+1].Text
				propEnd := len(paramToks)
				for j := i + 2; j < len(paramToks); j++ {
					if paramToks[j].Text == "," {
						propEnd = j
						break
					}
				}
				sig := Join(paramToks[i:propEnd])
				w.col.At(paramStart)
				w.col.Add(classPrefix+"."+propName, sig)
			}
		}
	}
}

func (w *tsWalker) extractReexportedNames(toks []Token) []string {
	var names []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Type == TokIdent && t.Text != "as" && t.Text != "type" {
			if i+2 < len(toks) && toks[i+1].Text == "as" {
				names = append(names, toks[i+2].Text)
				i += 2
			} else {
				names = append(names, t.Text)
			}
		}
	}
	return names
}

func (w *tsWalker) isTokenListLiteral(toks []Token) bool {
	if len(toks) == 0 {
		return false
	}
	if len(toks) == 1 {
		t := toks[0]
		if t.Type == TokStr {
			return !strings.Contains(t.Text, "${")
		}
		if t.Type == TokIdent {
			return t.Text == "true" || t.Text == "false" || t.Text == "null" || isNumeric(t.Text)
		}
	}
	return false
}

func (w *tsWalker) isLiteralInit(start, end int) bool {
	s := w.s
	toks := Tokenize(s, start, end)
	return w.isTokenListLiteral(toks)
}

func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && c != '.' && c != '-' && c != '+' && c != 'x' && c != 'X' {
			return false
		}
	}
	return true
}

func (w *tsWalker) findStatementEnd(start, end int) int {
	s := w.s
	i := start
	for i < end {
		if s.K[i] == Code {
			c := s.B[i]
			if c == ';' {
				return i + 1
			}
			if c == '(' || c == '[' || c == '{' {
				cl, err := s.MatchClose(i)
				if err == nil && cl < end {
					i = cl + 1
					continue
				}
			}
			if c == '\n' {
				next := s.NextCode(i + 1)
				if next >= end {
					return end
				}
				nextWord, _ := w.readWord(next, end)
				switch nextWord {
				case "export", "import", "const", "let", "var", "function", "class", "interface", "type", "enum", "declare", "namespace", "module", "abstract", "async":
					return i + 1
				}
			}
		}
		i++
	}
	return end
}

func (w *tsWalker) skipStatement(start, end int) int {
	s := w.s
	i := start
	for i < end {
		if s.K[i] == Code {
			c := s.B[i]
			if c == ';' {
				return i + 1
			}
			if c == '(' || c == '[' || c == '{' {
				cl, err := s.MatchClose(i)
				if err == nil && cl < end {
					i = cl + 1
					continue
				}
			}
			if c == '\n' {
				next := s.NextCode(i + 1)
				if next >= end {
					return end
				}
				nextWord, _ := w.readWord(next, end)
				switch nextWord {
				case "export", "import", "const", "let", "var", "function", "class", "interface", "type", "enum", "declare", "namespace", "module":
					return i + 1
				}
			}
		}
		i++
	}
	return end
}

func (w *tsWalker) readWord(start, end int) (string, int) {
	s := w.s
	i := s.NextCode(start)
	if i >= end {
		return "", end
	}
	c := s.B[i]
	if !isIdentByte(c) {
		return "", i
	}
	wordStart := i
	for i < end && isIdentByte(s.B[i]) {
		i++
	}
	return string(s.B[wordStart:i]), i
}

func (w *tsWalker) extractTypeName(start, end int) string {
	s := w.s
	i := s.NextCode(start)
	word, _ := w.readWord(i, end)
	return word
}

func (w *tsWalker) extractNameBeforeParen(start, parenIdx int) string {
	s := w.s
	toks := Tokenize(s, start, parenIdx)
	if len(toks) == 0 {
		return ""
	}
	return toks[len(toks)-1].Text
}

func (w *tsWalker) extractFieldNameAndEqual(start, end int) (name string, eqIdx int) {
	s := w.s
	eqIdx = -1
	cutEnd := end
	for i := start; i < end; i++ {
		if s.K[i] == Code && s.B[i] == '=' {
			eqIdx = i
			cutEnd = i
			break
		}
	}
	toks := Tokenize(s, start, cutEnd)
	if len(toks) == 0 {
		return "", eqIdx
	}
	name = toks[0].Text
	return name, eqIdx
}

func (w *tsWalker) findParenRange(start, end int) (int, int) {
	s := w.s
	for i := start; i < end; i++ {
		if s.K[i] == Code && s.B[i] == '(' {
			cl, err := s.MatchClose(i)
			if err == nil && cl <= end {
				return i, cl + 1
			}
		}
	}
	return -1, -1
}
