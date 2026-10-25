// Package symbols extracts public API signatures per file.
package symbols

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strings"

	"scribex/pkg/symbols/scan"
)

var ErrUnsupported = errors.New("symbols: unsupported language")

const (
	maxFileSize = 20 * 1024 * 1024 // 20 MB
)

// Supported reports whether scribex analyses this file extension.
func Supported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".kt", ".kts", ".java", ".py":
		return true
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		return true
	}
	// Check for .d.ts
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".d.ts") {
		return true
	}
	return false
}

// Extract returns key -> canonical signature. For a Supported path, any problem returns (nil, error);
// never a partial map, never a panic. For an unsupported extension it returns (nil, ErrUnsupported).
func Extract(path string, src []byte) (res map[string]string, retErr error) {
	if !Supported(path) {
		return nil, ErrUnsupported
	}
	if len(src) > maxFileSize {
		return nil, errors.New("symbols: file exceeds 20 MB limit")
	}

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("symbols: panic during extraction: %v", r)
		}
	}()

	ext := strings.ToLower(filepath.Ext(path))
	lower := strings.ToLower(path)

	if ext == ".go" {
		return extractGo(path, src)
	}

	if ext == ".kt" || ext == ".kts" {
		return scan.ScanKotlin(src)
	}

	if ext == ".java" {
		return scan.ScanJava(src)
	}

	if ext == ".py" {
		return scan.ScanPython(src)
	}

	if strings.HasSuffix(lower, ".d.ts") || ext == ".ts" || ext == ".tsx" || ext == ".mts" ||
		ext == ".cts" || ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" {
		return scan.ScanTS(src)
	}

	return nil, ErrUnsupported
}

func extractGo(path string, src []byte) (map[string]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string)
	pr := func(n any) string {
		var b bytes.Buffer
		printer.Fprint(&b, fset, n)
		return b.String()
	}

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				t := pr(d.Recv.List[0].Type)
				t = strings.TrimPrefix(t, "*")
				// Strip generic type parameters in the key, e.g. Box[T] -> Box
				if idx := strings.Index(t, "["); idx >= 0 {
					t = t[:idx]
				}
				if !ast.IsExported(t) {
					// Methods on unexported receivers excluded
					continue
				}
				name = t + "." + name
			}
			d.Body = nil
			d.Doc = nil
			out[name] = cleanGoSig(pr(d))

		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}

			var lastType ast.Expr
			var lastValues []ast.Expr

			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						s.Doc, s.Comment = nil, nil
						if st, ok := s.Type.(*ast.StructType); ok && st.Fields != nil {
							var expFields []*ast.Field
							for _, f := range st.Fields.List {
								if f.Names == nil {
									// embedded field
									expFields = append(expFields, f)
								} else {
									var expNames []*ast.Ident
									for _, n := range f.Names {
										if n.IsExported() {
											expNames = append(expNames, n)
										}
									}
									if len(expNames) > 0 {
										fClone := *f
										fClone.Names = expNames
										expFields = append(expFields, &fClone)
									}
								}
							}
							stClone := *st
							stClone.Fields = &ast.FieldList{List: expFields}
							sClone := *s
							sClone.Type = &stClone
							s = &sClone
						}
						out[s.Name.Name] = cleanGoSig("type " + pr(s))
					}

				case *ast.ValueSpec:
					if d.Tok == token.CONST {
						// Track type and values for iota groups
						if s.Type != nil {
							lastType = s.Type
						}
						if len(s.Values) > 0 {
							lastValues = s.Values
						}

						for idx, n := range s.Names {
							if !n.IsExported() {
								continue
							}
							sig := "const " + n.Name
							if s.Type != nil {
								sig += " " + pr(s.Type)
							} else if lastType != nil {
								sig += " " + pr(lastType)
							}

							valExpr := ""
							if idx < len(s.Values) {
								valExpr = pr(s.Values[idx])
							} else if idx < len(lastValues) {
								valExpr = pr(lastValues[idx])
							}
							if valExpr != "" {
								sig += " = " + valExpr
							}
							out[n.Name] = sig
						}
					} else if d.Tok == token.VAR {
						// Var: var Name Type (declared type only; if no declared type, name only)
						for _, n := range s.Names {
							if !n.IsExported() {
								continue
							}
							sig := "var " + n.Name
							if s.Type != nil {
								sig += " " + pr(s.Type)
							}
							out[n.Name] = sig
						}
					}
				}
			}
		}
	}

	return out, nil
}

func cleanGoSig(s string) string {
	src, err := scan.Lex(scan.Go, []byte(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	toks := scan.Tokenize(src, 0, len(src.B))
	if len(toks) == 0 {
		return strings.TrimSpace(s)
	}
	return scan.Join(toks)
}
