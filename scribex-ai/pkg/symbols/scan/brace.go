package scan

import (
	"errors"
	"fmt"
	"strings"
)

// Symbol represents an extracted declaration with rich detail.
type Symbol struct {
	Key  string
	Kind string
	Sig  string
	Line int
	Doc  string
}

// Decl represents an extracted declaration.
type Decl struct {
	Key        string
	Sig        string
	ParamTypes []string // canonical parameter types for overload keys
	Offset     int      // byte offset of the declaration header start
	Doc        string   // optional pre-extracted doc (e.g. Python docstring)
}

// Collector collects declarations and builds the canonical public contract map.
type Collector struct {
	decls     []Decl
	curOffset int
	curDoc    string
}

// At records the byte offset of the header start for the next Add.
func (c *Collector) At(offset int) {
	c.curOffset = offset
	c.curDoc = ""
}

// WithDoc sets the doc comment for the next Add.
func (c *Collector) WithDoc(doc string) {
	c.curDoc = doc
}

// Add appends a declaration to the collector.
func (c *Collector) Add(key, sig string, paramTypes ...string) {
	c.decls = append(c.decls, Decl{
		Key:        key,
		Sig:        sig,
		ParamTypes: paramTypes,
		Offset:     c.curOffset,
		Doc:        c.curDoc,
	})
	c.curDoc = ""
}

// Finalize produces the public contract map:
//   - Each plain key Class.name contains all overload signatures in source order joined with \n.
//   - When there is more than one declaration of that key and ParamTypes are provided,
//     additionally emits Class.name(T1,T2) per overload.
//   - Duplicate non-overload keys are joined with \n.
func (c *Collector) Finalize() map[string]string {
	out := make(map[string]string)
	if len(c.decls) == 0 {
		return out
	}

	// Group by key preserving source order
	keyGroups := make(map[string][]Decl)
	var orderedKeys []string

	for _, d := range c.decls {
		if _, exists := keyGroups[d.Key]; !exists {
			orderedKeys = append(orderedKeys, d.Key)
		}
		keyGroups[d.Key] = append(keyGroups[d.Key], d)
	}

	for _, k := range orderedKeys {
		decls := keyGroups[k]
		if len(decls) == 1 {
			out[k] = decls[0].Sig
		} else {
			// Multiple declarations for the same key
			sigs := make([]string, len(decls))
			hasOverloads := false
			for i, d := range decls {
				sigs[i] = d.Sig
				if d.ParamTypes != nil {
					hasOverloads = true
				}
			}
			out[k] = strings.Join(sigs, "\n")

			if hasOverloads {
				for _, d := range decls {
					overloadKey := fmt.Sprintf("%s(%s)", k, strings.Join(d.ParamTypes, ","))
					out[overloadKey] = d.Sig
				}
			}
		}
	}

	return out
}

// FinalizeDetailed produces the list of detailed Symbols:
//   - One entry per plain key in source order
//   - Overload typed keys (Class.name(T1)) are not repeated; overload signatures are joined with \n
//   - Line is 1-based start line of the declaration header
//   - Doc is extracted from preceding comments or python docstrings
func (c *Collector) FinalizeDetailed(s *Source, lang Lang) []Symbol {
	if len(c.decls) == 0 {
		return nil
	}

	keyGroups := make(map[string][]Decl)
	var orderedKeys []string

	for _, d := range c.decls {
		if _, exists := keyGroups[d.Key]; !exists {
			orderedKeys = append(orderedKeys, d.Key)
		}
		keyGroups[d.Key] = append(keyGroups[d.Key], d)
	}

	res := make([]Symbol, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		decls := keyGroups[k]
		first := decls[0]

		var sig string
		if len(decls) == 1 {
			sig = first.Sig
		} else {
			sigs := make([]string, len(decls))
			for i, d := range decls {
				sigs[i] = d.Sig
			}
			sig = strings.Join(sigs, "\n")
		}

		line := 1
		if s != nil {
			line = s.LineOf(first.Offset) + 1
		}

		doc := first.Doc
		if doc == "" && s != nil {
			doc = PrecedingDoc(s, first.Offset)
		}

		kind := DeriveKind(lang, k, first.Sig)

		res = append(res, Symbol{
			Key:  k,
			Kind: kind,
			Sig:  sig,
			Line: line,
			Doc:  doc,
		})
	}

	return res
}

const maxHeaderSize = 64 * 1024 // 64 KB

// FindBraceHeaderEnd finds the end index of a declaration header in a brace language.
// Starting at startIndex, it scans Code tokens until the first Code '{' or ';' at paren/bracket depth 0.
// It jumps over '()' and '[]' groups using MatchClose and handles '<>' generic parameters.
func FindBraceHeaderEnd(s *Source, startIndex int) (int, error) {
	n := len(s.B)
	i := startIndex
	for i < n {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == '{' || c == ';' {
			if i-startIndex > maxHeaderSize {
				return -1, errors.New("symbols: declaration header exceeds 64 KB")
			}
			return i, nil
		}
		if c == '(' || c == '[' {
			closeIdx, err := s.MatchClose(i)
			if err != nil {
				return -1, err
			}
			i = closeIdx + 1
			continue
		}
		if c == '<' {
			// Only treat as generic args if '<' directly follows an identifier
			prev := s.PrevCode(i)
			if prev >= 0 && isIdentByte(s.B[prev]) {
				closeIdx := matchAngleClose(s, i)
				if closeIdx > i {
					i = closeIdx + 1
					continue
				}
			}
		}
		i++
	}

	if i-startIndex > maxHeaderSize {
		return -1, errors.New("symbols: declaration header exceeds 64 KB")
	}
	return i, nil
}

// matchAngleClose finds matching '>' for a generic argument list starting at '<' at index i.
// It looks ahead for balanced '<' and '>' before hitting '{', ';', or '='.
// If unbalanced or hitting '{', ';', '=', returns -1 (treated as operator).
func matchAngleClose(s *Source, i int) int {
	depth := 1
	for j := i + 1; j < len(s.B); j++ {
		if s.K[j] != Code {
			continue
		}
		c := s.B[j]
		if c == '{' || c == ';' || c == '=' {
			return -1
		}
		if c == '(' || c == '[' {
			cl, err := s.MatchClose(j)
			if err != nil {
				return -1
			}
			j = cl
			continue
		}
		if c == '<' {
			depth++
		} else if c == '>' {
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// ExtractParamTypes parses canonical parameter types from a parameter list '( ... )'.
// It returns a list of parameter types (e.g. ["Int", "String..."]).
// Parameter names, default values, annotations, and modifiers are stripped,
// while 'vararg' / '...' is kept.
func ExtractParamTypes(s *Source, paramListStart, paramListEnd int) []string {
	if paramListStart >= paramListEnd {
		return nil
	}
	// Strip enclosing '(' and ')'
	start := s.NextCode(paramListStart + 1)
	end := paramListEnd
	prev := s.PrevCode(end)
	if prev >= start && s.B[prev] == ')' {
		end = prev
	}

	if start >= end {
		return []string{} // empty parameter list ()
	}

	// Split parameters by top-level commas ','
	var params [][]Token
	currentParamStart := start

	i := start
	for i < end {
		if s.K[i] != Code {
			i++
			continue
		}
		c := s.B[i]
		if c == '(' || c == '[' || c == '{' {
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
		if c == ',' {
			toks := Tokenize(s, currentParamStart, i)
			if len(toks) > 0 {
				params = append(params, toks)
			}
			currentParamStart = i + 1
		}
		i++
	}
	if currentParamStart < end {
		toks := Tokenize(s, currentParamStart, end)
		if len(toks) > 0 {
			params = append(params, toks)
		}
	}

	var result []string
	for _, ptoks := range params {
		ptype := extractSingleParamType(ptoks)
		if ptype != "" {
			result = append(result, ptype)
		}
	}
	return result
}

func extractSingleParamType(tokens []Token) string {
	if len(tokens) == 0 {
		return ""
	}

	// Check if this is Kotlin/TS parameter: `[vararg] name: Type [= default]`
	// or Java parameter: `[@Anno] [final] Type... name`
	colonIdx := -1
	for i, t := range tokens {
		if t.Text == ":" {
			colonIdx = i
			break
		}
	}

	if colonIdx >= 0 {
		// Kotlin / TS style: type is after ':' up to '=' or end
		typeToks := tokens[colonIdx+1:]
		eqIdx := -1
		for i, t := range typeToks {
			if t.Text == "=" {
				eqIdx = i
				break
			}
		}
		if eqIdx >= 0 {
			typeToks = typeToks[:eqIdx]
		}
		// Check if leading token had 'vararg'
		hasVararg := false
		for i := 0; i < colonIdx; i++ {
			if tokens[i].Text == "vararg" {
				hasVararg = true
				break
			}
		}
		joined := Join(typeToks)
		if hasVararg {
			return "vararg " + joined
		}
		return joined
	}

	// Java / C style: modifiers then Type then name
	// Drop annotations (@...), modifiers (final), name is last token
	// If varargs: Type... name
	var filtered []Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Text == "@" {
			// skip annotation and any paren
			if i+1 < len(tokens) && tokens[i+1].Type == TokIdent {
				i++
				if i+1 < len(tokens) && tokens[i+1].Text == "(" {
					// skip paren
					depth := 1
					i += 2
					for i < len(tokens) && depth > 0 {
						if tokens[i].Text == "(" {
							depth++
						} else if tokens[i].Text == ")" {
							depth--
						}
						i++
					}
					i--
				}
			}
			continue
		}
		if t.Text == "final" {
			continue
		}
		filtered = append(filtered, t)
	}

	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) == 1 {
		return filtered[0].Text
	}
	// Last token is parameter name; preceding tokens form the type
	typeToks := filtered[:len(filtered)-1]
	return Join(typeToks)
}
