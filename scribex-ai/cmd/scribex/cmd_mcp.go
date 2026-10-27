package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"scribex/pkg/mcp"
)

func kv(s string, wrap bool) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	m := map[string]string{}
	for _, p := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", p)
		}
		if wrap {
			v = "${" + v + "}"
		}
		m[k] = v
	}
	return m, nil
}

func mcpCmd(root string, a []string) error {
	if len(a) == 0 {
		return fmt.Errorf("mcp add|list|remove|check|export|serve")
	}
	if a[0] == "serve" {
		return mcp.Serve(os.Stdin, os.Stdout, root)
	}
	f, err := mcp.Load(root)
	if err != nil {
		return err
	}
	switch a[0] {
	case "add":
		if len(a) != 2 {
			return fmt.Errorf("mcp add <name> -preset p | -url u | -cmd c")
		}
		s := mcp.Server{}
		if *preset != "" {
			p, ok := mcp.Presets[*preset]
			if !ok {
				return fmt.Errorf("unknown preset; have %s", strings.Join(mcp.PresetNames(), ", "))
			}
			s = p
		}
		if *mcpURL != "" {
			s.URL = *mcpURL
		}
		if *mcpExe != "" {
			s.Command = *mcpExe
		}
		if *mcpArgs != "" {
			s.Args = strings.Split(*mcpArgs, ",")
		}
		env, err := kv(*mcpEnv, true)
		if err != nil {
			return err
		}
		hdr, err := kv(*mcpHdr, false)
		if err != nil {
			return err
		}
		if s.Env == nil && env != nil {
			s.Env = map[string]string{}
		}
		for k, v := range env {
			s.Env[k] = v
		}
		if s.Headers == nil && hdr != nil {
			s.Headers = map[string]string{}
		}
		for k, v := range hdr {
			s.Headers[k] = v
		}
		if err := s.Validate(); err != nil {
			return err
		}
		f.Servers[a[1]] = s
		if err := mcp.Save(root, f); err != nil {
			return err
		}
		if m := s.Missing(); len(m) > 0 {
			fmt.Println("added; set before use:", strings.Join(m, ", "), "(scribex secrets set NAME)")
		} else {
			fmt.Println("added", a[1])
		}
	case "remove":
		if len(a) != 2 {
			return fmt.Errorf("mcp remove <name>")
		}
		delete(f.Servers, a[1])
		return mcp.Save(root, f)
	case "list", "check":
		var ns []string
		for n := range f.Servers {
			ns = append(ns, n)
		}
		sort.Strings(ns)
		bad := 0
		for _, n := range ns {
			s := f.Servers[n]
			status := "ok"
			if err := s.Validate(); err != nil {
				status, bad = err.Error(), bad+1
			} else if m := s.Missing(); len(m) > 0 {
				status, bad = "unset: "+strings.Join(m, ","), bad+1
			}
			fmt.Printf("%-18s %s%s  [%s]\n", n, s.Command, s.URL, status)
		}
		if a[0] == "check" && bad > 0 {
			return fmt.Errorf("%d server(s) not ready", bad)
		}
	case "export":
		b, _ := os.ReadFile(mcp.Path(root))
		os.Stdout.Write(b) // drop-in for a client's .mcp.json
	default:
		return fmt.Errorf("unknown mcp command %q", a[0])
	}
	return nil
}
