package scan

import (
	"strings"
)

// TokenType indicates whether a token is an identifier, string literal, or punctuation.
type TokenType uint8

const (
	TokIdent TokenType = iota // identifier, keyword, number, unicode
	TokStr                    // string, char, template, regex literal
	TokPunct                  // punctuation byte or multi-byte punctuation
)

// Token represents a token extracted from a declaration header.
type Token struct {
	Type TokenType
	Text string
}

// Tokenize extracts tokens from the specified range [start, end) of a Source.
// Comments and whitespace are discarded. Str tokens are kept verbatim.
// Identifiers, numbers, and multi-byte punctuation tokens are extracted according to SPEC P0-2.
func Tokenize(s *Source, start, end int) []Token {
	if start < 0 {
		start = 0
	}
	if end > len(s.B) {
		end = len(s.B)
	}
	if start >= end {
		return nil
	}

	var tokens []Token
	b := s.B
	k := s.K

	i := start
	for i < end {
		// Skip comments
		if k[i] == Comment {
			i++
			continue
		}

		// String literals (Str) - contiguous Str bytes form a single Token
		if k[i] == Str {
			strStart := i
			for i < end && k[i] == Str {
				i++
			}
			tokens = append(tokens, Token{
				Type: TokStr,
				Text: string(b[strStart:i]),
			})
			continue
		}

		// Code bytes
		c := b[i]
		// Skip whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		// Backtick identifier (Kotlin `identifier with spaces`)
		if c == '`' {
			btStart := i
			i++
			for i < end && k[i] == Code && b[i] != '`' && b[i] != '\n' {
				i++
			}
			if i < end && k[i] == Code && b[i] == '`' {
				i++
			}
			tokens = append(tokens, Token{
				Type: TokIdent,
				Text: string(b[btStart:i]),
			})
			continue
		}

		// Identifiers, keywords, numbers, and unicode identifiers ([A-Za-z0-9_$] or byte >= 0x80)
		if isIdentByte(c) {
			idStart := i
			i++
			for i < end && k[i] == Code && isIdentByte(b[i]) {
				i++
			}
			tokens = append(tokens, Token{
				Type: TokIdent,
				Text: string(b[idStart:i]),
			})
			continue
		}

		// Multi-byte punctuation tokens:
		// "-> and =>, ::, ?., !!, .., ..., ==, !=, <=, >=, &&, ||, -> are multi-byte punctuation tokens (glue them)"
		mb, length := matchMultiPunct(b, k, i, end)
		if length > 1 {
			tokens = append(tokens, Token{
				Type: TokPunct,
				Text: mb,
			})
			i += length
			continue
		}

		// Single punctuation token
		tokens = append(tokens, Token{
			Type: TokPunct,
			Text: string(b[i : i+1]),
		})
		i++
	}

	return tokens
}

func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '_' || c == '$' || c >= 0x80
}

func isIdentStartByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$' || c >= 0x80
}

func matchMultiPunct(b []byte, k []Kind, i, end int) (string, int) {
	rem := end - i
	if rem >= 3 {
		if k[i] == Code && k[i+1] == Code && k[i+2] == Code {
			if b[i] == '.' && b[i+1] == '.' && b[i+2] == '.' {
				return "...", 3
			}
		}
	}
	if rem >= 2 {
		if k[i] == Code && k[i+1] == Code {
			c0, c1 := b[i], b[i+1]
			switch {
			case c0 == '-' && c1 == '>':
				return "->", 2
			case c0 == '=' && c1 == '>':
				return "=>", 2
			case c0 == ':' && c1 == ':':
				return "::", 2
			case c0 == '?' && c1 == '.':
				return "?.", 2
			case c0 == '!' && c1 == '!':
				return "!!", 2
			case c0 == '.' && c1 == '.':
				return "..", 2
			case c0 == '=' && c1 == '=':
				return "==", 2
			case c0 == '!' && c1 == '=':
				return "!=", 2
			case c0 == '<' && c1 == '=':
				return "<=", 2
			case c0 == '>' && c1 == '=':
				return ">=", 2
			case c0 == '&' && c1 == '&':
				return "&&", 2
			case c0 == '|' && c1 == '|':
				return "||", 2
			case c0 == '+' && c1 == '=':
				return "+=", 2
			case c0 == '-' && c1 == '=':
				return "-=", 2
			}
		}
	}
	return "", 1
}

// Join concatenates tokens into a canonical signature according to SPEC P0-2 formatting rules:
// - Insert exactly one space between two tokens unless:
//   - left is ( [ < . @ ! ~ (or ?., ::, ..)
//   - right is ) ] > , ; . ? (or ?., ::, !!, ..)
//   - ( or [ right after an identifier, >, ), or ] has no space
//   - < right after an identifier has no space
//   - right is : (no space before :; one space after)
//
// - Drop a trailing comma immediately before ) ] >.
// - Final result: no leading/trailing space, never contains \n or \t.
func Join(tokens []Token) string {
	if len(tokens) == 0 {
		return ""
	}

	// 1. Drop trailing comma immediately before ) ] >
	filtered := make([]Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Text == "," {
			if i+1 < len(tokens) {
				next := tokens[i+1].Text
				if next == ")" || next == "]" || next == ">" {
					continue
				}
			}
		}
		filtered = append(filtered, tokens[i])
	}
	tokens = filtered
	if len(tokens) == 0 {
		return ""
	}

	var sb strings.Builder
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if i > 0 {
			prev := tokens[i-1]
			if needsSpace(prev, tok) {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(tok.Text)
	}

	res := strings.TrimSpace(sb.String())
	// Ensure no \n or \t
	res = strings.ReplaceAll(res, "\n", " ")
	res = strings.ReplaceAll(res, "\t", " ")
	return res
}

func needsSpace(left, right Token) bool {
	lt := left.Text
	rt := right.Text

	// Left exceptions: ( [ < . @ ! ~ and multi-byte ?., ::, ..
	switch lt {
	case "(", "[", "<", ".", "@", "!", "~", "?.", "::", "..":
		return false
	}

	// Right exceptions: ) ] > , ; . ? and multi-byte ?., ::, !!, ..
	switch rt {
	case ")", "]", ">", ",", ";", ".", "?", "?.", "::", "!!", "..":
		return false
	case "...":
		if left.Type == TokIdent {
			return false
		}
	}

	// Right is : (no space before :, one space after)
	if rt == ":" {
		return false
	}
	if lt == ":" {
		return true
	}

	// ( or [ right after an identifier, >, ), or ] has no space
	if rt == "(" || rt == "[" {
		if left.Type == TokIdent || lt == ">" || lt == ")" || lt == "]" {
			return false
		}
	}

	// < right after an identifier has no space
	if rt == "<" {
		if left.Type == TokIdent {
			return false
		}
	}

	return true
}
