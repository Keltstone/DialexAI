package scan

import (
	"strings"
	"unicode"
)

// Valid symbol kinds (CARD_G1 Rule 3):
// func, method, class, interface, enum, enum_entry, record, object, trait,
// type, const, var, property, field, constructor, module, symbol (fallback).

// DeriveKind deterministically computes the symbol kind from the language, key, and signature.
func DeriveKind(lang Lang, key, sig string) string {
	switch lang {
	case Go:
		return deriveGoKind(key, sig)
	case Kotlin:
		return deriveKotlinKind(key, sig)
	case Java:
		return deriveJavaKind(key, sig)
	case TS:
		return deriveTSKind(key, sig)
	case Python:
		return derivePythonKind(key, sig)
	default:
		return "symbol"
	}
}

func deriveGoKind(key, sig string) string {
	s := strings.TrimSpace(sig)
	if strings.HasPrefix(s, "func (") {
		return "method"
	}
	if strings.HasPrefix(s, "func ") {
		return "func"
	}
	if strings.HasPrefix(s, "type ") {
		// type X struct or type X interface
		// check if definition contains struct or interface
		words := strings.Fields(s)
		for _, w := range words[2:] {
			if strings.HasPrefix(w, "struct") {
				return "class"
			}
			if strings.HasPrefix(w, "interface") {
				return "interface"
			}
		}
		return "type"
	}
	if strings.HasPrefix(s, "const ") {
		return "const"
	}
	if strings.HasPrefix(s, "var ") {
		return "var"
	}
	return "symbol"
}

func deriveKotlinKind(key, sig string) string {
	if strings.HasSuffix(key, ".<init>") {
		return "constructor"
	}

	toks := extractTokensFromSig(Kotlin, sig)
	idx := 0
	isConst := false

	// Skip annotations (@Name or @Name(...)) and modifiers
	for idx < len(toks) {
		next := skipAnnotations(toks, idx)
		if next > idx {
			idx = next
			continue
		}
		t := toks[idx].Text
		if t == "const" {
			isConst = true
			idx++
			continue
		}
		if isKtModifier(t) {
			idx++
			continue
		}
		break
	}

	if idx < len(toks) {
		kw := toks[idx].Text
		switch kw {
		case "fun":
			if strings.Contains(key, ".") {
				return "method"
			}
			return "func"
		case "class":
			return "class"
		case "object":
			return "object"
		case "interface":
			return "interface"
		case "typealias":
			return "type"
		case "val", "var":
			if isConst {
				return "const"
			}
			return "property"
		}
	}

	if strings.Contains(key, ".") {
		return "enum_entry"
	}
	return "symbol"
}

func isKtModifier(m string) bool {
	switch m {
	case "public", "private", "protected", "internal",
		"open", "abstract", "final", "sealed", "data", "inner", "enum",
		"annotation", "companion", "override", "suspend", "inline", "noinline",
		"crossinline", "infix", "operator", "tailrec", "external", "lateinit",
		"vararg", "value", "expect", "actual", "reified":
		return true
	}
	return false
}

func deriveJavaKind(key, sig string) string {
	if strings.HasSuffix(key, ".<init>") {
		return "constructor"
	}

	toks := extractTokensFromSig(Java, sig)
	idx := 0

	// Skip annotations (@Name or @Name(...)) and modifiers
	for idx < len(toks) {
		next := skipAnnotations(toks, idx)
		if next > idx {
			idx = next
			continue
		}
		t := toks[idx].Text
		if isJavaModifier(t) {
			idx++
			continue
		}
		break
	}

	if idx < len(toks) {
		kw := toks[idx].Text
		switch kw {
		case "class":
			return "class"
		case "interface", "@interface":
			return "interface"
		case "enum":
			return "enum"
		case "record":
			return "record"
		}
	}

	if strings.Contains(sig, "(") {
		return "method"
	}

	// If no type keywords, check if it's an enum entry or field
	// Enum constants: Color.RED where sig is "RED" or "RED(args)"
	if strings.Contains(key, ".") && idx < len(toks) {
		// If single word or word with '(', likely enum entry
		if idx == len(toks)-1 || (idx+1 < len(toks) && toks[idx+1].Text == "(") {
			return "enum_entry"
		}
	}

	return "field"
}

func isJavaModifier(m string) bool {
	switch m {
	case "public", "protected", "private", "static", "final", "abstract",
		"default", "synchronized", "native", "strictfp", "transient", "volatile":
		return true
	}
	return false
}

func deriveTSKind(key, sig string) string {
	if strings.HasSuffix(key, ".constructor") || strings.HasSuffix(key, ".new") {
		return "constructor"
	}

	toks := extractTokensFromSig(TS, sig)
	idx := 0

	// Skip export, declare, modifiers
	for idx < len(toks) {
		t := toks[idx].Text
		if t == "export" || t == "default" || t == "declare" || t == "abstract" ||
			t == "async" || t == "static" || t == "readonly" || t == "override" ||
			t == "public" || t == "protected" || t == "private" {
			idx++
			continue
		}
		break
	}

	if idx < len(toks) {
		kw := toks[idx].Text
		switch kw {
		case "function":
			return "func"
		case "class":
			return "class"
		case "interface":
			return "interface"
		case "type":
			return "type"
		case "enum":
			return "enum"
		case "const":
			return "const"
		case "let", "var":
			return "var"
		}
	}

	// Class or interface members
	if strings.Contains(key, ".") {
		if strings.Contains(sig, "(") {
			return "method"
		}
		// Enum members e.g. Color.RED where sig is "RED = 1" or "RED"
		if idx < len(toks) && !strings.Contains(sig, ":") {
			return "enum_entry"
		}
		return "property"
	}

	if strings.Contains(sig, "(") {
		return "func"
	}
	return "symbol"
}

func derivePythonKind(key, sig string) string {
	toks := extractTokensFromSig(Python, sig)
	idx := 0

	// Skip decorators
	for idx < len(toks) {
		next := skipAnnotations(toks, idx)
		if next > idx {
			idx = next
			continue
		}
		break
	}

	if idx < len(toks) && toks[idx].Text == "async" {
		idx++
	}

	if idx < len(toks) {
		kw := toks[idx].Text
		switch kw {
		case "def":
			if strings.Contains(key, ".") {
				return "method"
			}
			return "func"
		case "class":
			return "class"
		}
	}

	// Assignment or constant/var
	name := key
	if dot := strings.LastIndex(key, "."); dot >= 0 {
		name = key[dot+1:]
	}
	if isAllUpper(name) {
		return "const"
	}
	return "var"
}

func extractTokensFromSig(lang Lang, sig string) []Token {
	s, err := Lex(lang, []byte(sig))
	if err != nil {
		return nil
	}
	return Tokenize(s, 0, len(s.B))
}

func skipAnnotations(toks []Token, idx int) int {
	for idx < len(toks) && toks[idx].Text == "@" {
		idx++
		if idx < len(toks) && toks[idx].Type == TokIdent {
			idx++
		}
		if idx < len(toks) && toks[idx].Text == "(" {
			depth := 1
			idx++
			for idx < len(toks) && depth > 0 {
				if toks[idx].Text == "(" {
					depth++
				} else if toks[idx].Text == ")" {
					depth--
				}
				idx++
			}
		}
	}
	return idx
}

func isAllUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}
