package scan

import (
	"errors"
	"fmt"
	"strings"
)

// ScanJava extracts the public API symbols and canonical signatures from a Java source file.
func ScanJava(src []byte) (map[string]string, error) {
	s, err := Lex(Java, src)
	if err != nil {
		return nil, err
	}
	return ScanJavaSource(s)
}

// ScanJavaDetailed extracts detailed public API symbols from a Java source file.
func ScanJavaDetailed(src []byte) ([]Symbol, error) {
	s, err := Lex(Java, src)
	if err != nil {
		return nil, err
	}
	return ScanJavaSourceDetailed(s)
}

// ScanJavaSource extracts symbols from a lexed Java Source.
func ScanJavaSource(s *Source) (res map[string]string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during Java scanning: %v", r)
		}
	}()

	c := &Collector{}
	w := &javaWalker{s: s, col: c}
	err := w.scanScope(0, len(s.B), "", false)
	if err != nil {
		return nil, err
	}
	return c.Finalize(), nil
}

// ScanJavaSourceDetailed extracts detailed symbols from a lexed Java Source.
func ScanJavaSourceDetailed(s *Source) (res []Symbol, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during Java scanning: %v", r)
		}
	}()

	c := &Collector{}
	w := &javaWalker{s: s, col: c}
	err := w.scanScope(0, len(s.B), "", false)
	if err != nil {
		return nil, err
	}
	return c.FinalizeDetailed(s, Java), nil
}

type javaWalker struct {
	s   *Source
	col *Collector
}

var javaIgnoredAnnotations = map[string]bool{
	"Override":         true,
	"SuppressWarnings": true,
	"Generated":        true,
}

var javaTypeKeywords = map[string]bool{
	"class": true, "interface": true, "enum": true, "record": true, "@interface": true,
}

func (w *javaWalker) scanScope(scopeStart, scopeEnd int, prefix string, inInterface bool) error {
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
			i = w.skipToSemicolon(i, n)
			continue
		}
		if s.B[i] == 'p' && i+7 <= n && string(s.B[i:i+7]) == "package" && !isIdentByte(s.B[i+7]) {
			i = w.skipToSemicolon(i, n)
			continue
		}

		// Check for static/instance initializer block `{ ... }` or `static { ... }`
		if s.B[i] == '{' {
			cl, err := s.MatchClose(i)
			if err != nil {
				return err
			}
			i = cl + 1
			continue
		}
		if s.B[i] == 's' && i+6 <= n && string(s.B[i:i+6]) == "static" && !isIdentByte(s.B[i+6]) {
			nextNonSpace := s.NextCode(i + 6)
			if nextNonSpace < n && s.B[nextNonSpace] == '{' {
				cl, err := s.MatchClose(nextNonSpace)
				if err != nil {
					return err
				}
				i = cl + 1
				continue
			}
		}

		declStart := i
		isPublic := inInterface // Interface members are implicitly public unless private
		isPrivate := false
		isStatic := false
		isFinal := inInterface // Interface fields are implicitly static final
		if inInterface {
			isStatic = true
		}

		var collectedTokens []Token
		var typeKw string
		var typeKwEnd int

		// Parse annotations and modifiers
		for i < n {
			i = s.NextCode(i)
			if i >= n {
				break
			}
			c := s.B[i]

			// Annotation: @Name or @interface or @Name(...)
			if c == '@' {
				if i+10 <= n && string(s.B[i:i+10]) == "@interface" && !isIdentByte(s.B[i+10]) {
					typeKw = "@interface"
					typeKwEnd = i + 10
					i = typeKwEnd
					break
				}
				// Regular annotation
				annoName, annoEnd := w.readAnnotation(i, n)
				if !javaIgnoredAnnotations[annoName] {
					annoToks := Tokenize(s, i, annoEnd)
					collectedTokens = append(collectedTokens, annoToks...)
				}
				i = annoEnd
				continue
			}

			word, wordEnd := w.readWord(i, n)
			if word == "" {
				break
			}

			if word == "public" {
				isPublic = true
				collectedTokens = append(collectedTokens, Token{TokIdent, word})
				i = wordEnd
				continue
			}
			if word == "private" {
				isPrivate = true
				isPublic = false
				collectedTokens = append(collectedTokens, Token{TokIdent, word})
				i = wordEnd
				continue
			}
			if word == "protected" {
				isPublic = false
				collectedTokens = append(collectedTokens, Token{TokIdent, word})
				i = wordEnd
				continue
			}
			if word == "static" {
				isStatic = true
				collectedTokens = append(collectedTokens, Token{TokIdent, word})
				i = wordEnd
				continue
			}
			if word == "final" {
				isFinal = true
				collectedTokens = append(collectedTokens, Token{TokIdent, word})
				i = wordEnd
				continue
			}
			if word == "abstract" || word == "default" || word == "native" || word == "synchronized" || word == "strictfp" || word == "transient" || word == "volatile" || word == "sealed" || word == "non-sealed" {
				collectedTokens = append(collectedTokens, Token{TokIdent, word})
				i = wordEnd
				continue
			}

			// Check if type keyword
			if javaTypeKeywords[word] {
				typeKw = word
				typeKwEnd = wordEnd
				i = wordEnd
				break
			}

			// Non-modifier word: could be start of member type or constructor name or generic <T>
			break
		}

		if isPrivate {
			isPublic = false
		}

		if typeKw != "" {
			// Type declaration: class, interface, enum, record, @interface
			headerEnd, err := FindBraceHeaderEnd(s, declStart)
			if err != nil {
				return err
			}
			typeName := w.extractTypeName(typeKwEnd, headerEnd)
			fullKey := typeName
			if prefix != "" {
				fullKey = prefix + "." + typeName
			}

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
				sigToks := w.buildJavaSignature(s, declStart, headerEnd)
				w.col.At(declStart)
				w.col.Add(fullKey, Join(sigToks))

				if hasBody {
					if typeKw == "enum" {
						err := w.scanEnumBody(headerEnd+1, bodyClose, fullKey)
						if err != nil {
							return err
						}
					} else {
						isSubIface := (typeKw == "interface" || typeKw == "@interface")
						err := w.scanScope(headerEnd+1, bodyClose, fullKey, isSubIface)
						if err != nil {
							return err
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
		}

		// Member declaration (method, constructor, or field)
		memberNameStart := i
		headerEnd, err := FindBraceHeaderEnd(s, declStart)
		if err != nil {
			return err
		}

		// Find whether this is a method/constructor (contains '(') or field
		parenIdx := -1
		for j := memberNameStart; j < headerEnd; j++ {
			if s.K[j] == Code && s.B[j] == '(' {
				parenIdx = j
				break
			}
		}

		if parenIdx >= 0 {
			// Method or Constructor
			name, isCtor := w.extractMethodName(declStart, parenIdx, prefix)
			var key string
			if isCtor {
				key = prefix + ".<init>"
			} else if prefix != "" {
				key = prefix + "." + name
			} else {
				key = name
			}

			paramStart, paramEnd := w.findParenRange(parenIdx, headerEnd)
			paramTypes := ExtractParamTypes(s, paramStart, paramEnd)

			if isPublic {
				sigToks := w.buildJavaSignature(s, declStart, headerEnd)
				w.col.At(declStart)
				w.col.Add(key, Join(sigToks), paramTypes...)
			}

			// Skip body if method has '{'
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

		// Field declaration
		fieldName, eqIdx := w.extractFieldNameAndEqual(declStart, headerEnd)
		if fieldName != "" {
			key := fieldName
			if prefix != "" {
				key = prefix + "." + fieldName
			}

			if isPublic {
				var fieldSigToks []Token
				if isStatic && isFinal && eqIdx >= 0 {
					// Constant field: include '= <initializer>' up to ';' (max 4 KB)
					if headerEnd-eqIdx > 4*1024 {
						return errors.New("symbols: constant field expression exceeds 4 KB")
					}
					fieldSigToks = w.buildJavaSignature(s, declStart, headerEnd)
				} else {
					// Non-constant field: signature = modifiers + type + name (up to '=')
					cutEnd := headerEnd
					if eqIdx >= 0 {
						cutEnd = eqIdx
					}
					fieldSigToks = w.buildJavaSignature(s, declStart, cutEnd)
				}
				w.col.At(declStart)
				w.col.Add(key, Join(fieldSigToks))
			}
		}

		i = headerEnd + 1
	}

	return nil
}

func (w *javaWalker) scanEnumBody(bodyStart, bodyEnd int, enumKey string) error {
	s := w.s
	n := bodyEnd
	i := bodyStart

	// Enum constants before first ';'
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

		name, endName := w.readWord(i, n)
		if name == "" {
			i++
			continue
		}

		constEnd := endName
		nextCode := s.NextCode(endName)
		if nextCode < n && s.B[nextCode] == '(' {
			cl, err := s.MatchClose(nextCode)
			if err != nil {
				return err
			}
			constEnd = cl + 1
		}
		nextAfterArgs := s.NextCode(constEnd)
		if nextAfterArgs < n && s.B[nextAfterArgs] == '{' {
			cl, err := s.MatchClose(nextAfterArgs)
			if err != nil {
				return err
			}
			constEnd = cl + 1
		}

		constToks := Tokenize(s, i, constEnd)
		var clean []Token
		for _, t := range constToks {
			if t.Text == "{" {
				break
			}
			clean = append(clean, t)
		}
		cleanToks := clean
		w.col.At(i)
		w.col.Add(enumKey+"."+name, Join(cleanToks))

		i = constEnd
		skipComma := s.NextCode(i)
		if skipComma < n && s.B[skipComma] == ',' {
			i = skipComma + 1
		}
	}

	if i < n {
		return w.scanScope(i, bodyEnd, enumKey, false)
	}
	return nil
}

func (w *javaWalker) buildJavaSignature(s *Source, start, end int) []Token {
	// Tokenize range, dropping @Override, @SuppressWarnings, @Generated
	rawToks := Tokenize(s, start, end)
	var clean []Token
	for i := 0; i < len(rawToks); i++ {
		t := rawToks[i]
		if t.Text == "@" && i+1 < len(rawToks) {
			annoName := rawToks[i+1].Text
			if javaIgnoredAnnotations[annoName] {
				i++ // skip @
				// Also skip arguments if any: @Anno(...)
				if i+1 < len(rawToks) && rawToks[i+1].Text == "(" {
					depth := 1
					i += 2
					for i < len(rawToks) && depth > 0 {
						if rawToks[i].Text == "(" {
							depth++
						} else if rawToks[i].Text == ")" {
							depth--
						}
						i++
					}
					i--
				}
				continue
			}
		}
		clean = append(clean, t)
	}
	return clean
}

func (w *javaWalker) readAnnotation(start, end int) (name string, annoEnd int) {
	s := w.s
	i := start + 1
	for i < end && isIdentByte(s.B[i]) {
		i++
	}
	name = string(s.B[start+1 : i])
	next := s.NextCode(i)
	if next < end && s.B[next] == '(' {
		cl, err := s.MatchClose(next)
		if err == nil && cl < end {
			return name, cl + 1
		}
	}
	return name, i
}

func (w *javaWalker) readWord(start, end int) (string, int) {
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

func (w *javaWalker) extractTypeName(start, end int) string {
	s := w.s
	i := s.NextCode(start)
	word, _ := w.readWord(i, end)
	return word
}

func (w *javaWalker) extractMethodName(start, parenIdx int, enclosingPrefix string) (name string, isCtor bool) {
	s := w.s
	toks := Tokenize(s, start, parenIdx)
	if len(toks) == 0 {
		return "", false
	}
	name = toks[len(toks)-1].Text

	// Check if constructor: name matches enclosing class name (last component of prefix)
	enclosingSimpleName := enclosingPrefix
	if lastDot := strings.LastIndex(enclosingPrefix, "."); lastDot >= 0 {
		enclosingSimpleName = enclosingPrefix[lastDot+1:]
	}
	if enclosingSimpleName != "" && name == enclosingSimpleName {
		isCtor = true
	}
	return name, isCtor
}

func (w *javaWalker) extractFieldNameAndEqual(start, end int) (name string, eqIdx int) {
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
	name = toks[len(toks)-1].Text
	return name, eqIdx
}

func (w *javaWalker) skipToSemicolon(start, end int) int {
	s := w.s
	for i := start; i < end; i++ {
		if s.K[i] == Code && s.B[i] == ';' {
			return i + 1
		}
	}
	return end
}

func (w *javaWalker) findParenRange(start, end int) (int, int) {
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
