// Package symbols extracts public API signatures per file.
// ponytail: Go uses go/ast; other languages use line regexes. Tree-sitter needs cgo; swap in when regexes miss real code.
package symbols

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// Extract returns exported symbol name -> normalized signature. Unknown extensions yield nil.
func Extract(path string, src []byte) map[string]string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return goSyms(src)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		return lines(src, tsRe, 2, nil)
	case ".py":
		return lines(src, pyRe, 2, func(n string) bool { return strings.HasPrefix(n, "_") })
	case ".kt":
		return lines(src, ktRe, 2, nil)
	case ".java":
		m := lines(src, javaTypeRe, 2, nil)
		for k, v := range lines(src, javaMethodRe, 2, nil) {
			m[k] = v
		}
		return m
	}
	return nil
}

var (
	tsRe         = regexp.MustCompile(`^export\s+(?:default\s+)?((?:declare\s+)?(?:async\s+)?(?:function\*?|class|abstract class|interface|type|const|let|enum)\s+(\w+).*)$`)
	pyRe         = regexp.MustCompile(`^((?:async\s+)?(?:def|class)\s+(\w+).*)$`)
	ktRe         = regexp.MustCompile(`^\s*((?:(?:public|open|abstract|sealed|data|suspend|override|inline|enum|annotation|value)\s+)*(?:fun|class|interface|object)\s+(?:<[^>]*>\s*)?(?:\w+\.)?(\w+).*)$`)
	javaTypeRe   = regexp.MustCompile(`^\s*(public\s+(?:(?:static|final|abstract|sealed)\s+)*(?:class|interface|enum|record)\s+(\w+).*)$`)
	javaMethodRe = regexp.MustCompile(`^\s*(public\s+(?:(?:static|final|abstract|synchronized)\s+)*[\w<>\[\],? ]+?\s+(\w+)\s*\(.*)$`)
	ktPrivate    = regexp.MustCompile(`\b(private|internal|protected)\b`)
)

func lines(src []byte, re *regexp.Regexp, _ int, skip func(string) bool) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(string(src), "\n") {
		m := re.FindStringSubmatch(strings.TrimRight(l, " \t\r"))
		if m == nil || ktPrivate.MatchString(strings.SplitN(m[1], "(", 2)[0]) {
			continue
		}
		if skip != nil && skip(m[2]) {
			continue
		}
		out[m[2]] = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[1]), "{=:"))
	}
	return out
}

func goSyms(src []byte) map[string]string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	out := map[string]string{}
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
				t := strings.TrimPrefix(pr(d.Recv.List[0].Type), "*")
				if !ast.IsExported(t) {
					continue
				}
				name = t + "." + name
			}
			d.Body, d.Doc = nil, nil
			out[name] = pr(d)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						s.Doc, s.Comment = nil, nil
						out[s.Name.Name] = "type " + pr(s)
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.IsExported() {
							out[n.Name] = d.Tok.String() + " " + n.Name
						}
					}
				}
			}
		}
	}
	return out
}
