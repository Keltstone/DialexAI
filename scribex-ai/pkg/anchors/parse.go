package anchors

import (
	"errors"
	"regexp"
	"strings"
)

// Anchor is an anchor reference parsed from a markdown document.
type Anchor struct {
	Line int    // 1-based line number
	File string // relative file path
	Sym  string // symbol key
	Sig  string // raw sig= value ("" if absent)
}

// Single grammar for scribex anchors:
// File: [^)#]* (cannot contain ')' or '#')
// Sym: lazy \S+? matching up to the last ')' before the optional sig= or -->
// Whitespace inside Sym is not allowed.
var open = regexp.MustCompile(`^\s*<!--\s*@scribex:anchor\(([^)#]*)#(\S+?)\)(?:\s+sig=([0-9a-f]+))?\s*-->\s*$`)

// validateAnchor checks length, NUL, and character constraints.
func validateAnchor(file, sym string) error {
	if len(file) == 0 {
		return errors.New("empty file part")
	}
	if len(file) > 4096 {
		return errors.New("file path exceeds 4096 bytes")
	}
	if strings.ContainsRune(file, 0) {
		return errors.New("NUL in file path")
	}
	if strings.Contains(file, "\\\n") {
		return errors.New("backslash-newline in file path")
	}
	if len(sym) > 512 {
		return errors.New("symbol exceeds 512 bytes")
	}
	return nil
}

// InFence returns a slice of booleans where InFence(lines)[i] == true if line i
// is inside (or is the delimiter of) a fenced code block under simplified CommonMark rules.
func InFence(lines []string) []bool {
	res := make([]bool, len(lines))
	inFence := false
	var fenceChar byte
	fenceLen := 0

	for i, line := range lines {
		if !inFence {
			// Check if line opens a fence (max 3 leading spaces)
			spaces := 0
			for spaces < len(line) && line[spaces] == ' ' {
				spaces++
			}
			if spaces > 3 {
				continue
			}
			rest := line[spaces:]
			if len(rest) >= 3 && (rest[0] == '`' || rest[0] == '~') {
				ch := rest[0]
				count := 0
				for count < len(rest) && rest[count] == ch {
					count++
				}
				if count >= 3 {
					// CommonMark: backtick fence opener cannot contain backticks in info string
					if ch == '`' && strings.ContainsRune(rest[count:], '`') {
						continue
					}
					inFence = true
					fenceChar = ch
					fenceLen = count
					res[i] = true
				}
			}
		} else {
			res[i] = true
			// Check if line closes the fence (max 3 leading spaces)
			spaces := 0
			for spaces < len(line) && line[spaces] == ' ' {
				spaces++
			}
			if spaces > 3 {
				continue
			}
			rest := line[spaces:]
			count := 0
			for count < len(rest) && rest[count] == fenceChar {
				count++
			}
			if count >= fenceLen {
				// Must be followed only by spaces
				rem := rest[count:]
				onlySpaces := true
				for j := 0; j < len(rem); j++ {
					if rem[j] != ' ' {
						onlySpaces = false
						break
					}
				}
				if onlySpaces {
					inFence = false
					fenceChar = 0
					fenceLen = 0
				}
			}
		}
	}
	return res
}

// Parse extracts anchors outside code fences only, in order.
func Parse(md string) []Anchor {
	lines := strings.Split(md, "\n")
	inFence := InFence(lines)
	var out []Anchor
	for i, l := range lines {
		if inFence[i] {
			continue
		}
		m := open.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		file, sym, sig := m[1], m[2], m[3]
		if err := validateAnchor(file, sym); err != nil {
			continue
		}
		out = append(out, Anchor{
			Line: i + 1,
			File: file,
			Sym:  sym,
			Sig:  sig,
		})
	}
	return out
}
