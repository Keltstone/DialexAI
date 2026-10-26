package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"scribex/pkg/config"
	"scribex/pkg/wiki"
)

func wikiCmd(root string, a []string, out string) error {
	if len(a) == 0 {
		return fmt.Errorf("wiki new <title> | lint | search <q> | backlinks <page> | render -out dir")
	}
	c, err := config.Load(root)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, c.Wiki.Dir)
	if a[0] == "new" {
		if len(a) < 2 {
			return fmt.Errorf("wiki new <title>")
		}
		p, err := wiki.New(root, c.Wiki.Dir, strings.Join(a[1:], " "))
		if err == nil {
			fmt.Println("created", p)
		}
		return err
	}
	w, err := wiki.Load(dir)
	if err != nil {
		return err
	}
	switch a[0] {
	case "lint":
		p := w.Lint(c.Wiki.Home)
		for _, l := range p {
			fmt.Println(l)
		}
		if len(p) > 0 {
			return fmt.Errorf("%d wiki issue(s)", len(p))
		}
	case "search":
		for _, l := range w.Search(strings.Join(a[1:], " ")) {
			fmt.Println(l)
		}
	case "backlinks":
		if len(a) != 2 {
			return fmt.Errorf("wiki backlinks <page>")
		}
		for _, k := range w.Backlinks()[wiki.Key(a[1])] {
			fmt.Println(w.Pages[k].File)
		}
	case "render":
		if out == "" {
			return fmt.Errorf("wiki render needs -out dir")
		}
		return w.Render(out, c.Wiki.Home)
	default:
		return fmt.Errorf("unknown wiki command %q", a[0])
	}
	return nil
}
