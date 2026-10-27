package hunt

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"regexp"

	"sentinix/pkg/packs"
)

// Class is one vulnerability class a pack contributes for an entry-point kind.
type Class struct {
	ID   string `json:"id"`
	CWE  string `json:"cwe,omitempty"`
	Hint string `json:"hint,omitempty"` // one-line checklist item added to the prompt
}

// Pack is a shareable, signed set of extra classes: {"name","version","classes":{"component":[...]}}.
// Signing and trust reuse the rule-pack scheme (packs.Sign / packs.ReadKeys / -trust file).
type Pack struct {
	Name    string             `json:"name"`
	Version string             `json:"version"`
	Classes map[string][]Class `json:"classes"`
}

var idRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

// LoadPack verifies the signature, then validates ids and kinds. A pack can only add prompt text and class ids:
// ParsePack parses and validates an unsigned pack body.
func ParsePack(body []byte) (*Pack, error) {
	var p Pack
	if err := json.Unmarshal(body, &p); err != nil || p.Name == "" || p.Version == "" {
		return nil, fmt.Errorf("invalid hunt pack")
	}
	for kind, cs := range p.Classes {
		if _, ok := classes[kind]; !ok {
			if _, ok2 := baseKindClasses[kind]; !ok2 {
				return nil, fmt.Errorf("unknown entry-point kind %q", kind)
			}
		}
		for _, c := range cs {
			if !idRe.MatchString(c.ID) || len(c.Hint) > 300 {
				return nil, fmt.Errorf("invalid class %q", c.ID)
			}
		}
	}
	return &p, nil
}

// LoadPack verifies the signature, then validates ids and kinds. A pack can only add prompt text and class ids:
// it cannot change how findings gate (hunt findings never do).
func LoadPack(data []byte, trusted []ed25519.PublicKey) (*Pack, error) {
	body, err := packs.Verify(data, trusted)
	if err != nil {
		return nil, err
	}
	return ParsePack(body)
}

