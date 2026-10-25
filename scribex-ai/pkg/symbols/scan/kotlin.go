package scan

import (
	"errors"
	"fmt"
)

// ScanKotlin extracts the public API symbols and canonical signatures from a Kotlin source file.
func ScanKotlin(src []byte) (map[string]string, error) {
	s, err := Lex(Kotlin, src)
	if err != nil {
		return nil, err
	}
	return ScanKotlinSource(s)
}

// ScanKotlinSource extracts symbols from a lexed Kotlin Source.
func ScanKotlinSource(s *Source) (res map[string]string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during Kotlin scanning: %v", r)
		}
	}()

	c := &Collector{}
	w := &kotlinWalker{s: s, col: c}
	err := w.scanScope(0, len(s.B), "")
	if err != nil {
		return nil, err
	}
	return c.Finalize(), nil
}

type kotlinWalker struct {
	s   *Source
	col *Collector
}

var ktModifiers = map[string]bool{
	"public": true, "private": true, "protected": true, "internal": true,
	"open": true, "abstract": true, "final": true, "sealed": true,
	"data": true, "inner": true, "enum": true, "annotation": true,
	"companion": true, "override": true, "suspend": true, "inline": true,
	"noinline": true, "crossinline": true, "infix": true, "operator": true,
	"tailrec": true, "external": true, "const": true, "lateinit": true,
	"vararg": true, "value": true, "fun": true, "expect": true,
	"actual": true, "reified": true,
}

var ktDeclKeywords = map[string]bool{
	"class": true, "interface": true, "object": true, "typealias": true,
	"fun": true, "val": true, "var": true, "constructor": true, "init": true,
}

func (w *kotlinWalker) scanScope(scopeStart, scopeEnd int, prefix string) error {
	s := w.s
	n := scopeEnd
	i := scopeStart

	for i < n {
		i = s.NextCode(i)
		if i >= n {
			break
		}

		// Skip import and package statements
		if s.B[i] == 'i' && i+6 <= n && string(s.B[i:i+6]) == "import" && !isIdentByte(s.B[i+6]) {
			i = w.skipStatement(i, n)
			continue
		}
		if s.B[i] == 'p' && i+7 <= n && string(s.B[i:i+7]) == "package" && !isIdentByte(s.B[i+7]) {
			i = w.skipStatement(i, n)
			continue
		}

		// Read annotations and modifiers
		declStart := i
		vis := "public"
		var mods []string
		var isConst bool
		isCompanion := false

		for i < n {
			i = s.NextCode(i)
			if i >= n {
				break
			}
			c := s.B[i]
			if c == '@' {
				// Annotation: @Name or @file:Name or @get:Name or @Name(...)
				annoEnd := w.skipAnnotation(i, n)
				i = annoEnd
				continue
			}

			// Read token word
			tokWord, tokEnd := w.readWord(i, n)
			if tokWord == "" {
				break
			}

			if tokWord == "private" || tokWord == "internal" || tokWord == "protected" {
				vis = tokWord
				mods = append(mods, tokWord)
				i = tokEnd
				continue
			}
			if tokWord == "const" {
				isConst = true
				mods = append(mods, tokWord)
				i = tokEnd
				continue
			}
			if tokWord == "companion" {
				isCompanion = true
				mods = append(mods, tokWord)
				i = tokEnd
				continue
			}
			if ktModifiers[tokWord] && !ktDeclKeywords[tokWord] {
				mods = append(mods, tokWord)
				i = tokEnd
				continue
			}

			// Not a modifier or annotation -> stop modifier parsing
			break
		}

		i = s.NextCode(i)
		if i >= n {
			break
		}

		kw, kwEnd := w.readWord(i, n)
		if !ktDeclKeywords[kw] && !isCompanion {
			// Not a declaration start, advance to next line or token
			i = w.skipStatement(i, n)
			continue
		}

		isPublic := (vis == "public")

		switch kw {
		case "init":
			// skip init block
			openBrace := w.findOpenBrace(kwEnd, n)
			if openBrace >= 0 {
				closeBrace, err := s.MatchClose(openBrace)
				if err != nil {
					return err
				}
				i = closeBrace + 1
			} else {
				i = kwEnd
			}
			continue

		case "constructor":
			// Secondary constructor: Class.<init>
			headerEnd, err := FindBraceHeaderEnd(s, declStart)
			if err != nil {
				return err
			}
			if isPublic && prefix != "" {
				key := prefix + ".<init>"
				sigToks := Tokenize(s, declStart, headerEnd)
				paramStart, paramEnd := w.findParenRange(declStart, headerEnd)
				paramTypes := ExtractParamTypes(s, paramStart, paramEnd)
				w.col.Add(key, Join(sigToks), paramTypes...)
			}
			// Skip ctor body if present
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

		case "class", "interface", "object":
			// Type declaration (class, interface, object, or companion object)
			headerEnd := w.findTypeHeaderEnd(kwEnd, n)
			if headerEnd-declStart > maxHeaderSize {
				return errors.New("symbols: declaration header exceeds 64 KB")
			}

			typeName := w.extractTypeName(kwEnd, headerEnd)
			if isCompanion && typeName == "" {
				typeName = "Companion"
			}

			var fullKey string
			if prefix == "" {
				fullKey = typeName
			} else {
				fullKey = prefix + "." + typeName
			}

			sigToks := Tokenize(s, declStart, headerEnd)
			sig := Join(sigToks)

			hasBody := headerEnd < n && s.CodeAt(headerEnd) == '{'
			var bodyClose int
			if hasBody {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				bodyClose = cl
			}

			if isPublic {
				if isCompanion {
					// Kotlin companion object: reports members under Outer.Companion (or Outer.Name)
					if hasBody {
						err := w.scanScope(headerEnd+1, bodyClose, fullKey)
						if err != nil {
							return err
						}
					}
				} else {
					w.col.Add(fullKey, sig)
					if hasBody {
						// Check if enum class
						isEnum := false
						for _, m := range mods {
							if m == "enum" {
								isEnum = true
								break
							}
						}
						if isEnum {
							err := w.scanEnumBody(headerEnd+1, bodyClose, fullKey)
							if err != nil {
								return err
							}
						} else {
							err := w.scanScope(headerEnd+1, bodyClose, fullKey)
							if err != nil {
								return err
							}
						}
					}
				}
			}

			if hasBody {
				i = bodyClose + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "fun":
			// Function declaration
			headerEnd, hasExprBody, err := w.findFunHeaderEnd(kwEnd, n)
			if err != nil {
				return err
			}
			if headerEnd-declStart > maxHeaderSize {
				return errors.New("symbols: declaration header exceeds 64 KB")
			}

			funcName, receiverType := w.extractFunNameAndReceiver(kwEnd, headerEnd)
			var key string
			if receiverType != "" {
				// Extension function: Receiver.name
				key = receiverType + "." + funcName
			} else if prefix != "" {
				key = prefix + "." + funcName
			} else {
				key = funcName
			}

			paramStart, paramEnd := w.findParenRange(kwEnd, headerEnd)
			paramTypes := ExtractParamTypes(s, paramStart, paramEnd)

			if isPublic {
				sigToks := Tokenize(s, declStart, headerEnd)
				w.col.Add(key, Join(sigToks), paramTypes...)
			}

			// Skip body
			if hasExprBody {
				// Skip expression body up to end of statement
				i = w.skipExprBody(headerEnd, n)
			} else if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "val", "var":
			// Property declaration
			headerEnd, hasInit, err := w.findPropertyHeaderEnd(kwEnd, n, isConst)
			if err != nil {
				return err
			}

			propName, receiverType := w.extractPropNameAndReceiver(kwEnd, headerEnd)
			var key string
			if receiverType != "" {
				key = receiverType + "." + propName
			} else if prefix != "" {
				key = prefix + "." + propName
			} else {
				key = propName
			}

			if isPublic {
				sigToks := Tokenize(s, declStart, headerEnd)
				w.col.Add(key, Join(sigToks))
			}

			if hasInit && !isConst {
				i = w.skipExprBody(headerEnd, n)
			} else if headerEnd < n && s.CodeAt(headerEnd) == '{' {
				cl, err := s.MatchClose(headerEnd)
				if err != nil {
					return err
				}
				i = cl + 1
			} else {
				i = headerEnd + 1
			}
			continue

		case "typealias":
			// typealias Name<T> = Type
			headerEnd := w.skipStatement(kwEnd, n)
			name := w.extractTypeName(kwEnd, headerEnd)
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}
			if isPublic {
				sigToks := Tokenize(s, declStart, headerEnd)
				w.col.Add(key, Join(sigToks))
			}
			i = headerEnd
			continue
		}

		i++
	}
	return nil
}

func (w *kotlinWalker) scanEnumBody(bodyStart, bodyEnd int, enumKey string) error {
	s := w.s
	n := bodyEnd
	i := bodyStart

	// 1. Scan enum entries before first top-level ';' or end of body
	for i < n {
		i = s.NextCode(i)
		if i >= n {
			break
		}
		c := s.B[i]
		if c == ';' {
			i++
			break
		}
		if c == '}' {
			break
		}

		// Read entry name
		name, endName := w.readWord(i, n)
		if name == "" {
			i++
			continue
		}

		// Entry may have arguments (args) and class body { ... }
		entryEnd := endName
		nextCode := s.NextCode(endName)
		if nextCode < n && s.B[nextCode] == '(' {
			cl, err := s.MatchClose(nextCode)
			if err != nil {
				return err
			}
			entryEnd = cl + 1
		}
		nextAfterArgs := s.NextCode(entryEnd)
		if nextAfterArgs < n && s.B[nextAfterArgs] == '{' {
			cl, err := s.MatchClose(nextAfterArgs)
			if err != nil {
				return err
			}
			entryEnd = cl + 1
		}

		entryToks := Tokenize(s, i, entryEnd)
		// Clean entry sig: only entry name and optional (args), exclude { body }
		cleanToks := w.cleanEnumEntryTokens(entryToks)
		w.col.Add(enumKey+"."+name, Join(cleanToks))

		i = entryEnd
		// Skip comma if present
		skipComma := s.NextCode(i)
		if skipComma < n && s.B[skipComma] == ',' {
			i = skipComma + 1
		}
	}

	// 2. Scan remaining members of the enum class
	if i < n {
		return w.scanScope(i, bodyEnd, enumKey)
	}
	return nil
}

func (w *kotlinWalker) cleanEnumEntryTokens(tokens []Token) []Token {
	var clean []Token
	for _, t := range tokens {
		if t.Text == "{" {
			break
		}
		clean = append(clean, t)
	}
	return clean
}

func (w *kotlinWalker) skipStatement(start, end int) int {
	s := w.s
	i := start
	for i < end {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == ';' {
			return i + 1
		}
		if c == '\n' {
			// check if next non-space byte continues statement
			next := s.NextCode(i + 1)
			if next >= end {
				return end
			}
			nc := s.B[next]
			if nc != '.' && nc != ':' && nc != ',' {
				return i + 1
			}
		}
		if c == '(' || c == '[' || c == '{' {
			cl, err := s.MatchClose(i)
			if err == nil && cl < end {
				i = cl + 1
				continue
			}
		}
		i++
	}
	return end
}

func (w *kotlinWalker) skipAnnotation(start, end int) int {
	s := w.s
	i := start + 1
	// Read annotation name (can be file:Name or get:Name or just Name)
	for i < end && (isIdentByte(s.B[i]) || s.B[i] == ':') {
		i++
	}
	next := s.NextCode(i)
	if next < end && s.B[next] == '(' {
		cl, err := s.MatchClose(next)
		if err == nil && cl < end {
			return cl + 1
		}
	}
	return i
}

func (w *kotlinWalker) readWord(start, end int) (string, int) {
	s := w.s
	i := s.NextCode(start)
	if i >= end {
		return "", end
	}
	c := s.B[i]
	if c == '`' {
		// Backtick identifier
		btStart := i
		i++
		for i < end && s.K[i] == Code && s.B[i] != '`' && s.B[i] != '\n' {
			i++
		}
		if i < end && s.B[i] == '`' {
			i++
		}
		return string(s.B[btStart:i]), i
	}
	if !isIdentByte(c) {
		return "", i
	}
	wordStart := i
	for i < end && isIdentByte(s.B[i]) {
		i++
	}
	return string(s.B[wordStart:i]), i
}

func (w *kotlinWalker) findOpenBrace(start, end int) int {
	s := w.s
	for i := start; i < end; i++ {
		if s.K[i] == Code && s.B[i] == '{' {
			return i
		}
	}
	return -1
}

func (w *kotlinWalker) findParenRange(start, end int) (int, int) {
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

func (w *kotlinWalker) findTypeHeaderEnd(start, end int) int {
	s := w.s
	i := start
	for i < end {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == '{' || c == ';' {
			return i
		}
		if c == '(' || c == '[' {
			cl, err := s.MatchClose(i)
			if err == nil && cl < end {
				i = cl + 1
				continue
			}
		}
		if c == '<' {
			cl := matchAngleClose(s, i)
			if cl > i && cl < end {
				i = cl + 1
				continue
			}
		}
		if c == '\n' {
			// check continuation
			next := s.NextCode(i + 1)
			if next >= end {
				return end
			}
			nc := s.B[next]
			if nc == '{' {
				return next
			}
			prev := s.PrevCode(i)
			if prev >= start {
				pc := s.B[prev]
				if pc == ':' || pc == ',' || nc == ':' || nc == ',' {
					i++
					continue
				}
			}
			// If next line starts with a declaration or modifier, header ended
			nextWord, _ := w.readWord(next, end)
			if ktDeclKeywords[nextWord] || ktModifiers[nextWord] || s.B[next] == '@' {
				return i
			}
		}
		i++
	}
	return end
}

func (w *kotlinWalker) extractTypeName(start, end int) string {
	s := w.s
	// First identifier after start
	i := s.NextCode(start)
	word, _ := w.readWord(i, end)
	return word
}

func (w *kotlinWalker) findFunHeaderEnd(start, end int) (headerEnd int, hasExprBody bool, err error) {
	s := w.s
	i := start
	// Must find parameter list '(' ... ')'
	for i < end {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == '(' {
			cl, err := s.MatchClose(i)
			if err != nil {
				return -1, false, err
			}
			i = cl + 1
			break
		}
		if c == '<' {
			cl := matchAngleClose(s, i)
			if cl > i && cl < end {
				i = cl + 1
				continue
			}
		}
		i++
	}

	// After ')', scan for return type, then '{' or '='
	for i < end {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == '{' {
			return i, false, nil
		}
		if c == '=' {
			return i, true, nil
		}
		if c == ';' {
			return i, false, nil
		}
		if c == '(' || c == '[' {
			cl, err := s.MatchClose(i)
			if err == nil && cl < end {
				i = cl + 1
				continue
			}
		}
		if c == '<' {
			cl := matchAngleClose(s, i)
			if cl > i && cl < end {
				i = cl + 1
				continue
			}
		}
		if c == '\n' {
			next := s.NextCode(i + 1)
			if next < end {
				nc := s.B[next]
				if nc == ':' || nc == '.' || nc == ',' || nc == '=' || nc == '{' {
					i++
					continue
				}
				prev := s.PrevCode(i)
				if prev >= start {
					pc := s.B[prev]
					if pc == ':' || pc == ',' || pc == '(' || pc == '<' {
						i++
						continue
					}
				}
			}
			return i, false, nil
		}
		i++
	}
	return end, false, nil
}

func (w *kotlinWalker) findPropertyHeaderEnd(start, end int, isConst bool) (headerEnd int, hasInit bool, err error) {
	s := w.s
	i := start
	for i < end {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == '=' {
			if isConst {
				// const val includes the expression up to newline or ';'
				eqIdx := i
				i++
				for i < end && s.B[i] != '\n' && s.B[i] != ';' {
					i++
				}
				if i-eqIdx > 4*1024 {
					return -1, false, errors.New("symbols: const val expression exceeds 4 KB")
				}
				return i, false, nil
			}
			return i, true, nil
		}
		if c == ';' || c == '{' {
			return i, false, nil
		}
		// 'by' delegate
		if c == 'b' && i+2 <= end && s.B[i+1] == 'y' && !isIdentByte(s.B[i+2]) {
			return i, false, nil
		}
		if c == '(' || c == '[' {
			cl, err := s.MatchClose(i)
			if err == nil && cl < end {
				i = cl + 1
				continue
			}
		}
		if c == '<' {
			cl := matchAngleClose(s, i)
			if cl > i && cl < end {
				i = cl + 1
				continue
			}
		}
		if c == '\n' {
			next := s.NextCode(i + 1)
			if next < end {
				nc := s.B[next]
				if nc == ':' || nc == '=' || nc == '.' || nc == ',' {
					i++
					continue
				}
			}
			return i, false, nil
		}
		i++
	}
	return end, false, nil
}

func (w *kotlinWalker) skipExprBody(start, end int) int {
	s := w.s
	i := start
	for i < end {
		if s.K[i] == Code {
			c := s.B[i]
			if c == ';' {
				return i + 1
			}
			if c == '{' || c == '(' || c == '[' {
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
				nc := s.B[next]
				if nc == '}' {
					return next
				}
				// If next line begins with declaration start or modifier, expression ended
				nextWord, _ := w.readWord(next, end)
				if ktDeclKeywords[nextWord] || ktModifiers[nextWord] || s.B[next] == '@' {
					return i + 1
				}
			}
		}
		i++
	}
	return end
}

func (w *kotlinWalker) extractFunNameAndReceiver(start, end int) (name, receiver string) {
	s := w.s
	// First find '('
	parenIdx := -1
	for i := start; i < end; i++ {
		if s.K[i] == Code && s.B[i] == '(' {
			parenIdx = i
			break
		}
	}
	if parenIdx < 0 {
		return "", ""
	}

	// Between start and parenIdx:
	// May have type parameters <T>, receiver Receiver., and name
	toks := Tokenize(s, start, parenIdx)
	if len(toks) == 0 {
		return "", ""
	}

	// Drop type parameters <...> at beginning
	if toks[0].Text == "<" {
		depth := 0
		idx := 0
		for idx < len(toks) {
			if toks[idx].Text == "<" {
				depth++
			} else if toks[idx].Text == ">" {
				depth--
				if depth == 0 {
					idx++
					break
				}
			}
			idx++
		}
		toks = toks[idx:]
	}

	if len(toks) == 0 {
		return "", ""
	}

	// Name is the last identifier
	name = toks[len(toks)-1].Text

	// Check if there was a '.' before name indicating extension receiver
	dotIdx := -1
	for i := len(toks) - 2; i >= 0; i-- {
		if toks[i].Text == "." {
			dotIdx = i
			break
		}
	}

	if dotIdx >= 0 {
		// Receiver tokens are before dotIdx
		recvToks := toks[:dotIdx]
		// Strip type parameters from receiver key, e.g. List<T> -> List
		var cleanRecv []Token
		for _, t := range recvToks {
			if t.Text == "<" {
				break
			}
			cleanRecv = append(cleanRecv, t)
		}
		receiver = Join(cleanRecv)
	}

	return name, receiver
}

func (w *kotlinWalker) extractPropNameAndReceiver(start, end int) (name, receiver string) {
	s := w.s
	// Stop before ':' or '=' or 'by'
	cutIdx := end
	for i := start; i < end; i++ {
		if s.K[i] == Code {
			c := s.B[i]
			if c == ':' || c == '=' {
				cutIdx = i
				break
			}
			if c == 'b' && i+2 <= end && s.B[i+1] == 'y' && !isIdentByte(s.B[i+2]) {
				cutIdx = i
				break
			}
		}
	}

	toks := Tokenize(s, start, cutIdx)
	if len(toks) == 0 {
		return "", ""
	}

	name = toks[len(toks)-1].Text

	dotIdx := -1
	for i := len(toks) - 2; i >= 0; i-- {
		if toks[i].Text == "." {
			dotIdx = i
			break
		}
	}

	if dotIdx >= 0 {
		recvToks := toks[:dotIdx]
		var cleanRecv []Token
		for _, t := range recvToks {
			if t.Text == "<" {
				break
			}
			cleanRecv = append(cleanRecv, t)
		}
		receiver = Join(cleanRecv)
	}

	return name, receiver
}
