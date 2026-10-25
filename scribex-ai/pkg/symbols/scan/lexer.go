package scan

import (
	"bytes"
	"errors"
	"fmt"
)

// Lang identifies the source language being lexed.
type Lang uint8

const (
	Go Lang = iota
	Kotlin
	Java
	TS
	Python
)

// Kind classifies a byte's role in the source.
type Kind uint8

const (
	Code    Kind = iota
	Comment      // Comment bytes
	Str          // String, char, template, or regex content, including quotes/delimiters
)

// Source represents preprocessed and lexed source bytes.
type Source struct {
	B         []byte
	K         []Kind
	LineStart []int
}

const (
	maxFileSize = 20 * 1024 * 1024 // 20 MB
	maxNesting  = 500
)

// Lex classifies every byte of src for the given language.
// It strips UTF-8 BOM, normalizes CRLF and lone CR to LF, and validates that
// comments, strings, templates, and brackets are well-formed.
func Lex(lang Lang, src []byte) (*Source, error) {
	if len(src) > maxFileSize {
		return nil, errors.New("symbols: file exceeds 20 MB limit")
	}

	// 1. Strip leading UTF-8 BOM if present.
	if bytes.HasPrefix(src, []byte{0xef, 0xbb, 0xbf}) {
		src = src[3:]
	}

	// 2. Normalize CRLF and lone CR to LF.
	b := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		if src[i] == '\r' {
			b = append(b, '\n')
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
		} else {
			b = append(b, src[i])
		}
	}

	k := make([]Kind, len(b))

	// 3. Compute LineStart.
	lineStart := []int{0}
	for i, c := range b {
		if c == '\n' {
			lineStart = append(lineStart, i+1)
		}
	}

	s := &Source{
		B:         b,
		K:         k,
		LineStart: lineStart,
	}

	// 4. Lex according to language rules.
	var err error
	switch lang {
	case Kotlin:
		err = s.lexKotlin()
	case Java:
		err = s.lexJava()
	case TS:
		err = s.lexTS()
	case Python:
		err = s.lexPython()
	case Go:
		err = s.lexGo()
	default:
		return nil, errors.New("symbols: unsupported language in lexer")
	}
	if err != nil {
		return nil, err
	}

	// 5. Validate brackets and nesting depth for brace languages
	if lang == Kotlin || lang == Java || lang == TS {
		if err := s.validateBrackets(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Source) validateBrackets() error {
	stack := make([]byte, 0, 16)
	for i := 0; i < len(s.B); i++ {
		if s.K[i] != Code {
			continue
		}
		c := s.B[i]
		switch c {
		case '{':
			if len(stack) >= maxNesting {
				return errors.New("symbols: nesting depth exceeded 500")
			}
			stack = append(stack, '}')
		case '(':
			if len(stack) >= maxNesting {
				return errors.New("symbols: nesting depth exceeded 500")
			}
			stack = append(stack, ')')
		case '[':
			if len(stack) >= maxNesting {
				return errors.New("symbols: nesting depth exceeded 500")
			}
			stack = append(stack, ']')
		case '}', ')', ']':
			if len(stack) == 0 {
				return fmt.Errorf("symbols: unexpected closing bracket '%c' at index %d", c, i)
			}
			top := stack[len(stack)-1]
			if c != top {
				return fmt.Errorf("symbols: mismatched bracket: expected '%c', got '%c' at index %d", top, c, i)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return errors.New("symbols: unbalanced brackets at EOF")
	}
	return nil
}

// CodeAt returns s.B[i] if index i is valid and s.K[i] == Code, else 0.
func (s *Source) CodeAt(i int) byte {
	if i < 0 || i >= len(s.B) || s.K[i] != Code {
		return 0
	}
	return s.B[i]
}

// NextCode returns the index of the next non-whitespace Code byte at or after i,
// or len(s.B) if none exists.
func (s *Source) NextCode(i int) int {
	if i < 0 {
		i = 0
	}
	for ; i < len(s.B); i++ {
		if s.K[i] == Code {
			c := s.B[i]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				return i
			}
		}
	}
	return len(s.B)
}

// PrevCode returns the index of the previous non-whitespace Code byte strictly before i,
// or -1 if none exists.
func (s *Source) PrevCode(i int) int {
	if i > len(s.B) {
		i = len(s.B)
	}
	for j := i - 1; j >= 0; j-- {
		if s.K[j] == Code {
			c := s.B[j]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				return j
			}
		}
	}
	return -1
}

// MatchClose returns the index of the matching closer for '{', '(', or '[' at index i.
// Only Code bytes are considered. An error is returned if unbalanced or depth > 500.
// MatchClose is iterative with an explicit stack, never recursive.
func (s *Source) MatchClose(i int) (int, error) {
	if i < 0 || i >= len(s.B) || s.K[i] != Code {
		return -1, fmt.Errorf("symbols: no opening bracket at index %d", i)
	}
	open := s.B[i]
	var expectedClose byte
	switch open {
	case '{':
		expectedClose = '}'
	case '(':
		expectedClose = ')'
	case '[':
		expectedClose = ']'
	default:
		return -1, fmt.Errorf("symbols: byte '%c' is not an opening bracket", open)
	}

	stack := make([]byte, 0, 16)
	stack = append(stack, expectedClose)

	for j := i + 1; j < len(s.B); j++ {
		if s.K[j] != Code {
			continue
		}
		c := s.B[j]
		switch c {
		case '{':
			if len(stack) >= maxNesting {
				return -1, errors.New("symbols: nesting depth exceeded 500")
			}
			stack = append(stack, '}')
		case '(':
			if len(stack) >= maxNesting {
				return -1, errors.New("symbols: nesting depth exceeded 500")
			}
			stack = append(stack, ')')
		case '[':
			if len(stack) >= maxNesting {
				return -1, errors.New("symbols: nesting depth exceeded 500")
			}
			stack = append(stack, ']')
		case '}', ')', ']':
			if len(stack) == 0 {
				return -1, fmt.Errorf("symbols: unexpected closing bracket '%c' at index %d", c, j)
			}
			top := stack[len(stack)-1]
			if c != top {
				return -1, fmt.Errorf("symbols: mismatched bracket: expected '%c', got '%c' at index %d", top, c, j)
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return j, nil
			}
		}
	}

	return -1, errors.New("symbols: unbalanced bracket at EOF")
}

// LineOf returns the 0-indexed line number containing byte offset i.
func (s *Source) LineOf(i int) int {
	if i < 0 {
		return 0
	}
	low, high := 0, len(s.LineStart)-1
	for low <= high {
		mid := (low + high) / 2
		if s.LineStart[mid] <= i {
			if mid == len(s.LineStart)-1 || s.LineStart[mid+1] > i {
				return mid
			}
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return 0
}

// --- Kotlin Lexer ---
func (s *Source) lexKotlin() error {
	b := s.B
	n := len(b)
	i := 0
	for i < n {
		c := b[i]
		if c == '/' && i+1 < n {
			if b[i+1] == '/' {
				// Line comment
				for i < n && b[i] != '\n' {
					s.K[i] = Comment
					i++
				}
				continue
			}
			if b[i+1] == '*' {
				// Nested block comment
				depth := 1
				s.K[i] = Comment
				s.K[i+1] = Comment
				i += 2
				for i < n && depth > 0 {
					if b[i] == '/' && i+1 < n && b[i+1] == '*' {
						depth++
						s.K[i] = Comment
						s.K[i+1] = Comment
						i += 2
					} else if b[i] == '*' && i+1 < n && b[i+1] == '/' {
						depth--
						s.K[i] = Comment
						s.K[i+1] = Comment
						i += 2
					} else {
						s.K[i] = Comment
						i++
					}
				}
				if depth > 0 {
					return errors.New("symbols: unterminated block comment")
				}
				continue
			}
		}

		if c == '"' {
			if i+2 < n && b[i+1] == '"' && b[i+2] == '"' {
				// Raw string """..."""
				s.K[i] = Str
				s.K[i+1] = Str
				s.K[i+2] = Str
				i += 3
				closed := false
				for i < n {
					if b[i] == '"' && i+2 < n && b[i+1] == '"' && b[i+2] == '"' {
						s.K[i] = Str
						s.K[i+1] = Str
						s.K[i+2] = Str
						i += 3
						// Consume any extra quotes as part of the raw string closing/content
						for i < n && b[i] == '"' {
							s.K[i] = Str
							i++
						}
						closed = true
						break
					}
					s.K[i] = Str
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated raw string literal")
				}
				continue
			}

			// Regular string "..." with ${...} templates and escapes
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				ch := b[i]
				if ch == '\n' {
					return errors.New("symbols: unescaped newline in string literal")
				}
				if ch == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						if b[i] == '\n' {
							return errors.New("symbols: unescaped newline in string literal")
						}
						s.K[i] = Str
						i++
					}
					continue
				}
				if ch == '$' && i+1 < n {
					if b[i+1] == '{' {
						// Template ${ ... }
						s.K[i] = Str
						s.K[i+1] = Str
						i += 2
						braceDepth := 1
						for i < n && braceDepth > 0 {
							tch := b[i]
							if tch == '{' {
								braceDepth++
								s.K[i] = Str
								i++
							} else if tch == '}' {
								braceDepth--
								s.K[i] = Str
								i++
							} else if tch == '"' {
								// Nested string inside template
								if i+2 < n && b[i+1] == '"' && b[i+2] == '"' {
									s.K[i], s.K[i+1], s.K[i+2] = Str, Str, Str
									i += 3
									rawClosed := false
									for i < n {
										if b[i] == '"' && i+2 < n && b[i+1] == '"' && b[i+2] == '"' {
											s.K[i], s.K[i+1], s.K[i+2] = Str, Str, Str
											i += 3
											rawClosed = true
											break
										}
										s.K[i] = Str
										i++
									}
									if !rawClosed {
										return errors.New("symbols: unterminated raw string in template")
									}
								} else {
									s.K[i] = Str
									i++
									innerClosed := false
									for i < n {
										if b[i] == '\n' {
											return errors.New("symbols: unescaped newline in template string")
										}
										if b[i] == '\\' {
											s.K[i] = Str
											i++
											if i < n {
												s.K[i] = Str
												i++
											}
											continue
										}
										if b[i] == '"' {
											s.K[i] = Str
											i++
											innerClosed = true
											break
										}
										s.K[i] = Str
										i++
									}
									if !innerClosed {
										return errors.New("symbols: unterminated string in template")
									}
								}
							} else if tch == '\'' {
								// Char literal in template
								s.K[i] = Str
								i++
								charClosed := false
								for i < n && b[i] != '\n' {
									if b[i] == '\\' {
										s.K[i] = Str
										i++
										if i < n {
											s.K[i] = Str
											i++
										}
										continue
									}
									if b[i] == '\'' {
										s.K[i] = Str
										i++
										charClosed = true
										break
									}
									s.K[i] = Str
									i++
								}
								if !charClosed {
									return errors.New("symbols: unterminated char in template")
								}
							} else {
								s.K[i] = Str
								i++
							}
						}
						if braceDepth > 0 {
							return errors.New("symbols: unterminated template expression")
						}
						continue
					}
				}
				if ch == '"' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated string literal")
			}
			continue
		}

		if c == '\'' {
			// Char literal
			s.K[i] = Str
			i++
			closed := false
			for i < n && b[i] != '\n' {
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n && b[i] != '\n' {
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == '\'' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated char literal")
			}
			continue
		}

		// Otherwise Code
		s.K[i] = Code
		i++
	}
	return nil
}

// --- Java Lexer ---
func (s *Source) lexJava() error {
	b := s.B
	n := len(b)
	i := 0
	for i < n {
		c := b[i]
		if c == '/' && i+1 < n {
			if b[i+1] == '/' {
				for i < n && b[i] != '\n' {
					s.K[i] = Comment
					i++
				}
				continue
			}
			if b[i+1] == '*' {
				// Block comment (not nested in Java)
				s.K[i] = Comment
				s.K[i+1] = Comment
				i += 2
				closed := false
				for i+1 < n {
					if b[i] == '*' && b[i+1] == '/' {
						s.K[i] = Comment
						s.K[i+1] = Comment
						i += 2
						closed = true
						break
					}
					s.K[i] = Comment
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated block comment")
				}
				continue
			}
		}

		if c == '"' {
			if i+2 < n && b[i+1] == '"' && b[i+2] == '"' {
				// Java 15+ text block """..."""
				s.K[i] = Str
				s.K[i+1] = Str
				s.K[i+2] = Str
				i += 3
				closed := false
				for i < n {
					if b[i] == '\\' {
						s.K[i] = Str
						i++
						if i < n {
							s.K[i] = Str
							i++
						}
						continue
					}
					if b[i] == '"' && i+2 < n && b[i+1] == '"' && b[i+2] == '"' {
						s.K[i] = Str
						s.K[i+1] = Str
						s.K[i+2] = Str
						i += 3
						closed = true
						break
					}
					s.K[i] = Str
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated text block")
				}
				continue
			}

			// Single-line string "..."
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '\n' {
					return errors.New("symbols: unescaped newline in string literal")
				}
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						if b[i] == '\n' {
							return errors.New("symbols: unescaped newline in string literal")
						}
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == '"' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated string literal")
			}
			continue
		}

		if c == '\'' {
			// Char literal
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '\n' {
					return errors.New("symbols: unescaped newline in char literal")
				}
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						if b[i] == '\n' {
							return errors.New("symbols: unescaped newline in char literal")
						}
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == '\'' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated char literal")
			}
			continue
		}

		s.K[i] = Code
		i++
	}
	return nil
}

// --- TS/JS Lexer ---
func (s *Source) lexTS() error {
	b := s.B
	n := len(b)
	i := 0
	for i < n {
		c := b[i]
		if c == '/' && i+1 < n {
			if b[i+1] == '/' {
				for i < n && b[i] != '\n' {
					s.K[i] = Comment
					i++
				}
				continue
			}
			if b[i+1] == '*' {
				// Block comment (not nested in JS/TS)
				s.K[i] = Comment
				s.K[i+1] = Comment
				i += 2
				closed := false
				for i+1 < n {
					if b[i] == '*' && b[i+1] == '/' {
						s.K[i] = Comment
						s.K[i+1] = Comment
						i += 2
						closed = true
						break
					}
					s.K[i] = Comment
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated block comment")
				}
				continue
			}

			// Check if '/' starts a regex literal
			if s.isTSRegexStart(i) {
				s.K[i] = Str
				i++
				inBracket := false
				closed := false
				for i < n {
					rc := b[i]
					if rc == '\n' {
						return errors.New("symbols: unescaped newline in regex literal")
					}
					if rc == '\\' {
						s.K[i] = Str
						i++
						if i < n {
							if b[i] == '\n' {
								return errors.New("symbols: unescaped newline in regex literal")
							}
							s.K[i] = Str
							i++
						}
						continue
					}
					if rc == '[' {
						inBracket = true
						s.K[i] = Str
						i++
						continue
					}
					if rc == ']' && inBracket {
						inBracket = false
						s.K[i] = Str
						i++
						continue
					}
					if rc == '/' && !inBracket {
						s.K[i] = Str
						i++
						// Consume regex flags [a-z]*
						for i < n && (b[i] >= 'a' && b[i] <= 'z') {
							s.K[i] = Str
							i++
						}
						closed = true
						break
					}
					s.K[i] = Str
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated regex literal")
				}
				continue
			}
		}

		if c == '\'' || c == '"' {
			quote := c
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '\n' {
					return errors.New("symbols: unescaped newline in string literal")
				}
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						// Escaped newline is allowed in JS string literals
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == quote {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated string literal")
			}
			continue
		}

		if c == '`' {
			// Template literal `...${ code }...`
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				tch := b[i]
				if tch == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						s.K[i] = Str
						i++
					}
					continue
				}
				if tch == '$' && i+1 < n && b[i+1] == '{' {
					// Template expression
					s.K[i] = Str
					s.K[i+1] = Str
					i += 2
					braceDepth := 1
					for i < n && braceDepth > 0 {
						ech := b[i]
						if ech == '{' {
							braceDepth++
							s.K[i] = Str
							i++
						} else if ech == '}' {
							braceDepth--
							s.K[i] = Str
							i++
						} else if ech == '\'' || ech == '"' {
							inQuote := ech
							s.K[i] = Str
							i++
							qClosed := false
							for i < n {
								if b[i] == '\n' {
									return errors.New("symbols: unescaped newline in string inside template")
								}
								if b[i] == '\\' {
									s.K[i] = Str
									i++
									if i < n {
										s.K[i] = Str
										i++
									}
									continue
								}
								if b[i] == inQuote {
									s.K[i] = Str
									i++
									qClosed = true
									break
								}
								s.K[i] = Str
								i++
							}
							if !qClosed {
								return errors.New("symbols: unterminated string in template")
							}
						} else if ech == '`' {
							// Nested template literal
							s.K[i] = Str
							i++
							innerClosed := false
							for i < n {
								if b[i] == '\\' {
									s.K[i] = Str
									i++
									if i < n {
										s.K[i] = Str
										i++
									}
									continue
								}
								if b[i] == '`' {
									s.K[i] = Str
									i++
									innerClosed = true
									break
								}
								s.K[i] = Str
								i++
							}
							if !innerClosed {
								return errors.New("symbols: unterminated nested template literal")
							}
						} else {
							s.K[i] = Str
							i++
						}
					}
					if braceDepth > 0 {
						return errors.New("symbols: unterminated template expression")
					}
					continue
				}
				if tch == '`' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated template literal")
			}
			continue
		}

		s.K[i] = Code
		i++
	}
	return nil
}

// isTSRegexStart reports whether the '/' at index i starts a regex literal.
// Per SPEC P0-2:
// "a / starts a regex only if the previous significant Code token is one of
// ( , = : [ ! & | ? { } ; ~ ^ % * + - < > , or one of the keywords
// return typeof instanceof in of new delete void throw case do else yield await,
// or start of file; otherwise / is division."
func (s *Source) isTSRegexStart(i int) bool {
	if i+1 < len(s.B) && s.B[i+1] == '>' {
		// JSX self-closing tag />
		return false
	}
	prev := s.PrevCode(i)
	if prev < 0 {
		return true // start of file
	}
	pc := s.B[prev]
	if pc == '<' && prev == i-1 {
		// JSX closing tag </tag>
		return false
	}
	switch pc {
	case '(', ',', '=', ':', '[', '!', '&', '|', '?', '{', '}', ';', '~', '^', '%', '*', '+', '-', '<', '>':
		return true
	}

	// Check if previous token is an identifier / keyword
	if (pc >= 'a' && pc <= 'z') || (pc >= 'A' && pc <= 'Z') || (pc >= '0' && pc <= '9') || pc == '_' || pc == '$' {
		start := prev
		for start > 0 {
			prevChar := s.B[start-1]
			if s.K[start-1] == Code && ((prevChar >= 'a' && prevChar <= 'z') || (prevChar >= 'A' && prevChar <= 'Z') || (prevChar >= '0' && prevChar <= '9') || prevChar == '_' || prevChar == '$') {
				start--
			} else {
				break
			}
		}
		kw := string(s.B[start : prev+1])
		switch kw {
		case "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "throw", "case", "do", "else", "yield", "await":
			return true
		}
	}
	return false
}

// --- Python Lexer ---
func (s *Source) lexPython() error {
	b := s.B
	n := len(b)
	i := 0
	for i < n {
		c := b[i]
		if c == '#' {
			for i < n && b[i] != '\n' {
				s.K[i] = Comment
				i++
			}
			continue
		}

		// Check for string prefix (e.g. r, b, u, f, br, rb, fr, rf)
		prefixLen := 0
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			pl, isPrefix := checkPyStringPrefix(b, i)
			if isPrefix {
				prefixLen = pl
			}
		}

		quoteIdx := i + prefixLen
		if quoteIdx < n && (b[quoteIdx] == '\'' || b[quoteIdx] == '"') {
			quoteChar := b[quoteIdx]
			isTriple := quoteIdx+2 < n && b[quoteIdx+1] == quoteChar && b[quoteIdx+2] == quoteChar

			// Mark prefix bytes as Str
			for k := 0; k < prefixLen; k++ {
				s.K[i+k] = Str
			}
			i = quoteIdx

			if isTriple {
				s.K[i] = Str
				s.K[i+1] = Str
				s.K[i+2] = Str
				i += 3
				closed := false
				for i < n {
					if b[i] == '\\' {
						s.K[i] = Str
						i++
						if i < n {
							s.K[i] = Str
							i++
						}
						continue
					}
					if b[i] == quoteChar && i+2 < n && b[i+1] == quoteChar && b[i+2] == quoteChar {
						s.K[i] = Str
						s.K[i+1] = Str
						s.K[i+2] = Str
						i += 3
						closed = true
						break
					}
					s.K[i] = Str
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated triple-quoted string")
				}
				continue
			}

			// Single-quoted Python string
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						// Line continuation in string literal
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == '\n' {
					return errors.New("symbols: unescaped newline in single-quoted string")
				}
				if b[i] == quoteChar {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated string literal")
			}
			continue
		}

		s.K[i] = Code
		i++
	}
	return nil
}

func checkPyStringPrefix(b []byte, i int) (int, bool) {
	n := len(b)
	// Must be at the start of a token (preceded by non-identifier)
	if i > 0 {
		prev := b[i-1]
		if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') || prev == '_' {
			return 0, false
		}
	}

	// 2-letter prefixes: br, rb, fr, rf (case-insensitive)
	if i+2 < n && (b[i+2] == '\'' || b[i+2] == '"') {
		p0 := b[i]
		if p0 >= 'A' && p0 <= 'Z' {
			p0 += 'a' - 'A'
		}
		p1 := b[i+1]
		if p1 >= 'A' && p1 <= 'Z' {
			p1 += 'a' - 'A'
		}
		if (p0 == 'b' && p1 == 'r') || (p0 == 'r' && p1 == 'b') ||
			(p0 == 'f' && p1 == 'r') || (p0 == 'r' && p1 == 'f') {
			return 2, true
		}
	}

	// 1-letter prefixes: r, u, b, f (case-insensitive)
	if i+1 < n && (b[i+1] == '\'' || b[i+1] == '"') {
		p0 := b[i]
		if p0 >= 'A' && p0 <= 'Z' {
			p0 += 'a' - 'A'
		}
		if p0 == 'r' || p0 == 'u' || p0 == 'b' || p0 == 'f' {
			return 1, true
		}
	}

	return 0, false
}

// --- Go Lexer (basic support) ---
func (s *Source) lexGo() error {
	b := s.B
	n := len(b)
	i := 0
	for i < n {
		c := b[i]
		if c == '/' && i+1 < n {
			if b[i+1] == '/' {
				for i < n && b[i] != '\n' {
					s.K[i] = Comment
					i++
				}
				continue
			}
			if b[i+1] == '*' {
				s.K[i] = Comment
				s.K[i+1] = Comment
				i += 2
				closed := false
				for i+1 < n {
					if b[i] == '*' && b[i+1] == '/' {
						s.K[i] = Comment
						s.K[i+1] = Comment
						i += 2
						closed = true
						break
					}
					s.K[i] = Comment
					i++
				}
				if !closed {
					return errors.New("symbols: unterminated block comment")
				}
				continue
			}
		}

		if c == '`' {
			// Raw string
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '`' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated raw string")
			}
			continue
		}

		if c == '"' {
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '\n' {
					return errors.New("symbols: unescaped newline in string literal")
				}
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == '"' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated string literal")
			}
			continue
		}

		if c == '\'' {
			s.K[i] = Str
			i++
			closed := false
			for i < n {
				if b[i] == '\n' {
					return errors.New("symbols: unescaped newline in rune literal")
				}
				if b[i] == '\\' {
					s.K[i] = Str
					i++
					if i < n {
						s.K[i] = Str
						i++
					}
					continue
				}
				if b[i] == '\'' {
					s.K[i] = Str
					i++
					closed = true
					break
				}
				s.K[i] = Str
				i++
			}
			if !closed {
				return errors.New("symbols: unterminated rune literal")
			}
			continue
		}

		s.K[i] = Code
		i++
	}
	return nil
}
