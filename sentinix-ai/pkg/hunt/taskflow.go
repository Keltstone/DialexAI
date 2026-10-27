package hunt

import (
	"bytes"
	"crypto/ed25519"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sentinix/pkg/ai"
	"sentinix/pkg/packs"
)

//go:embed flows/*.json
var embeddedFlows embed.FS

// TaskDef defines one isolated, declarative task within a taskflow.
type TaskDef struct {
	Name         string   `json:"name"`
	Model        string   `json:"model"` // role: "discover", "classify", "validate", "poc", "report", or model name
	Prompt       string   `json:"prompt"`
	ForEach      string   `json:"for_each,omitempty"` // "entrypoints", "classes", "findings", or ""
	Passes       []string `json:"passes,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Budget       int      `json:"budget,omitempty"`
	MustComplete bool     `json:"must_complete,omitempty"`
	DependsOn    []string `json:"depends_on,omitempty"`
	FreshContext *bool    `json:"fresh_context,omitempty"`
}

// IsFreshContext returns whether the task runs in a sterile context (default true).
func (t TaskDef) IsFreshContext() bool {
	if t.FreshContext == nil {
		return true
	}
	return *t.FreshContext
}

// Taskflow defines an ordered sequence of declarative security audit tasks.
type Taskflow struct {
	Version     int               `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Globals     map[string]string `json:"globals,omitempty"`
	Tasks       []TaskDef         `json:"tasks"`
}

// ParseTaskflow parses JSON taskflow definition with strict schema validation.
func ParseTaskflow(data []byte) (*Taskflow, error) {
	var tf Taskflow
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tf); err != nil {
		return nil, fmt.Errorf("invalid taskflow: %w", err)
	}
	if tf.Version < 1 {
		return nil, errors.New("taskflow version must be >= 1")
	}
	if tf.Name == "" {
		return nil, errors.New("taskflow name is required")
	}
	if len(tf.Tasks) == 0 {
		return nil, errors.New("taskflow must have at least one task")
	}
	for i, t := range tf.Tasks {
		if t.Name == "" {
			return nil, fmt.Errorf("task[%d]: name is required", i)
		}
		if t.Prompt == "" {
			return nil, fmt.Errorf("task[%d] (%s): prompt template is required", i, t.Name)
		}
	}
	return &tf, nil
}

// LoadTaskflow loads a taskflow by name (from built-in flows) or from a file path.
func LoadTaskflow(nameOrPath string) (*Taskflow, error) {
	// 1. Check if it's a built-in flow name
	norm := strings.TrimSuffix(filepath.Base(nameOrPath), ".json")
	if data, err := embeddedFlows.ReadFile("flows/" + norm + ".json"); err == nil {
		return ParseTaskflow(data)
	}

	// 2. Otherwise load from local filesystem
	data, err := os.ReadFile(nameOrPath)
	if err != nil {
		return nil, fmt.Errorf("taskflow %q not found (built-in or file): %w", nameOrPath, err)
	}
	return ParseTaskflow(data)
}

// ListBuiltinTaskflows returns the names of all embedded taskflows.
func ListBuiltinTaskflows() []string {
	entries, _ := embeddedFlows.ReadDir("flows")
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names
}

// SignTaskflow signs a taskflow JSON using an ed25519 private key.
func SignTaskflow(flowData []byte, privKey ed25519.PrivateKey) ([]byte, error) {
	return packs.Sign(flowData, privKey)
}

// LoadSignedTaskflow verifies an ed25519 signature before parsing the taskflow.
func LoadSignedTaskflow(signedData []byte, trustedKeys []ed25519.PublicKey) (*Taskflow, error) {
	verifiedBody, err := packs.Verify(signedData, trustedKeys)
	if err != nil {
		return nil, fmt.Errorf("taskflow verification failed: %w", err)
	}
	return ParseTaskflow(verifiedBody)
}

// TemplateContext holds variables available during safe prompt template rendering.
type TemplateContext struct {
	Globals  map[string]string
	Entry    *Entry
	Code     string
	ClassID  string
	ClassCWE string
	Hint     string
	PassName string
	PassAsk  string
	ToolHelp string
	Advisory string
	Prev     map[string]string
	Finding  map[string]string
}

var (
	varPattern   = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)
	dangerSyntax = regexp.MustCompile(`(?i)(<\s*script|eval\s*\(|exec\s*\(|import\s+|include\s+)`)
)

// RenderTemplate safely evaluates prompt variables without code execution or file includes.
// Untrusted inputs (code, entry points, advisory text) are automatically wrapped as untrusted data.
func RenderTemplate(tmpl string, ctx TemplateContext) (string, error) {
	// Rejection of dangerous scripting injection syntax
	if dangerSyntax.MatchString(tmpl) {
		return "", errors.New("template contains prohibited code execution or script inclusion syntax")
	}

	rendered := varPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		varName := strings.TrimSpace(match[2 : len(match)-2])
		parts := strings.Split(varName, ".")

		switch parts[0] {
		case "globals":
			if len(parts) > 1 && ctx.Globals != nil {
				if v, ok := ctx.Globals[parts[1]]; ok {
					return v
				}
				if parts[1] == "advisory" && ctx.Advisory != "" {
					return ai.Untrusted(truncate(ctx.Advisory, 3000))
				}
			}
		case "entry":
			if ctx.Entry != nil {
				if len(parts) > 1 {
					switch parts[1] {
					case "name":
						return ctx.Entry.Name
					case "file":
						return ctx.Entry.File
					case "kinds":
						return strings.Join(ctx.Entry.Kinds, "+")
					case "code":
						return ai.Untrusted(ctx.Code)
					}
				}
			}
		case "class":
			if len(parts) > 1 {
				switch parts[1] {
				case "id":
					return ctx.ClassID
				case "cwe":
					return ctx.ClassCWE
				case "hint":
					if ctx.Hint != "" {
						return "Check specifically: " + ctx.Hint + "\n"
					}
					return ""
				}
			}
		case "pass":
			if len(parts) > 1 {
				switch parts[1] {
				case "name":
					return ctx.PassName
				case "ask":
					return ctx.PassAsk
				}
			}
		case "tools":
			if len(parts) > 1 && parts[1] == "help" {
				return ctx.ToolHelp
			}
		case "prev":
			if len(parts) > 1 && ctx.Prev != nil {
				if v, ok := ctx.Prev[parts[1]]; ok {
					return v
				}
			}
		case "finding":
			if len(parts) > 1 && ctx.Finding != nil {
				if v, ok := ctx.Finding[parts[1]]; ok {
					return v
				}
			}
		}
		return match // Keep unresolved variables or empty
	})

	return rendered, nil
}
