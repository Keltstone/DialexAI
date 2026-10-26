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

type Symbol = scan.Symbol

// ExtractDetailed returns public API symbols in source order with kind, signature, 1-based start line, and doc comment.
// Overload typed keys (Class.name(T1)) are not repeated; overload signatures are joined with \n in the plain key.
func ExtractDetailed(path string, src []byte) (res []Symbol, retErr error) {
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
		return extractGoDetailed(path, src)
	}

	if ext == ".kt" || ext == ".kts" {
		return scan.ScanKotlinDetailed(src)
	}

	if ext == ".java" {
		return scan.ScanJavaDetailed(src)
	}

	if ext == ".py" {
		return scan.ScanPythonDetailed(src)
	}

	if strings.HasSuffix(lower, ".d.ts") || ext == ".ts" || ext == ".tsx" || ext == ".mts" ||
		ext == ".cts" || ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" {
		return scan.ScanTSDetailed(src)
	}

	return nil, ErrUnsupported
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
	syms, err := extractGoDetailed(path, src)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(syms))
	for _, s := range syms {
		out[s.Key] = s.Sig
	}
	return out, nil
}

func extractGoDetailed(path string, src []byte) ([]Symbol, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	var res []Symbol
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
			kind := "func"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
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

			line := fset.Position(d.Pos()).Line
			var doc string
			if d.Doc != nil {
				doc = scan.CleanDoc(d.Doc.Text())
			}

			dClone := *d
			dClone.Body = nil
			dClone.Doc = nil
			sig := cleanGoSig(pr(&dClone))

			res = append(res, Symbol{
				Key:  name,
				Kind: kind,
				Sig:  sig,
				Line: line,
				Doc:  doc,
			})

		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}

			var lastType ast.Expr
			var lastValues []ast.Expr
			specIndex := 0

			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						line := fset.Position(s.Pos()).Line
						var doc string
						if s.Doc != nil {
							doc = scan.CleanDoc(s.Doc.Text())
						} else if d.Doc != nil && len(d.Specs) == 1 {
							doc = scan.CleanDoc(d.Doc.Text())
						}

						sClone := *s
						sClone.Doc, sClone.Comment = nil, nil
						if st, ok := sClone.Type.(*ast.StructType); ok && st.Fields != nil {
							var expFields []*ast.Field
							for _, fld := range st.Fields.List {
								if fld.Names == nil {
									// embedded field
									expFields = append(expFields, fld)
								} else {
									var expNames []*ast.Ident
									for _, n := range fld.Names {
										if n.IsExported() {
											expNames = append(expNames, n)
										}
									}
									if len(expNames) > 0 {
										fClone := *fld
										fClone.Names = expNames
										expFields = append(expFields, &fClone)
									}
								}
							}
							stClone := *st
							stClone.Fields = &ast.FieldList{List: expFields}
							sClone.Type = &stClone
						}

						sig := cleanGoSig("type " + pr(&sClone))
						kind := scan.DeriveKind(scan.Go, s.Name.Name, sig)

						res = append(res, Symbol{
							Key:  s.Name.Name,
							Kind: kind,
							Sig:  sig,
							Line: line,
							Doc:  doc,
						})
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
							line := fset.Position(n.Pos()).Line
							var doc string
							if s.Doc != nil {
								doc = scan.CleanDoc(s.Doc.Text())
							} else if d.Doc != nil && len(d.Specs) == 1 {
								doc = scan.CleanDoc(d.Doc.Text())
							}

							sig := "const " + n.Name
							if s.Type != nil {
								sig += " " + pr(s.Type)
							} else if lastType != nil {
								sig += " " + pr(lastType)
							}

							var valExpr ast.Expr
							if idx < len(s.Values) {
								valExpr = s.Values[idx]
							} else if idx < len(lastValues) {
								valExpr = lastValues[idx]
							}
							if valExpr != nil {
								sig += " = " + pr(valExpr)
								if containsIota(valExpr) {
									sig += fmt.Sprintf(" // iota=%d", specIndex)
								}
							}

							res = append(res, Symbol{
								Key:  n.Name,
								Kind: "const",
								Sig:  sig,
								Line: line,
								Doc:  doc,
							})
						}
						specIndex++
					} else if d.Tok == token.VAR {
						for _, n := range s.Names {
							if !n.IsExported() {
								continue
							}
							line := fset.Position(n.Pos()).Line
							var doc string
							if s.Doc != nil {
								doc = scan.CleanDoc(s.Doc.Text())
							} else if d.Doc != nil && len(d.Specs) == 1 {
								doc = scan.CleanDoc(d.Doc.Text())
							}

							sig := "var " + n.Name
							if s.Type != nil {
								sig += " " + pr(s.Type)
							}

							res = append(res, Symbol{
								Key:  n.Name,
								Kind: "var",
								Sig:  sig,
								Line: line,
								Doc:  doc,
							})
						}
					}
				}
			}
		}
	}

	return res, nil
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

func containsIota(e ast.Expr) bool {
	if e == nil {
		return false
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "iota" {
			found = true
			return false
		}
		return !found
	})
	return found
}
