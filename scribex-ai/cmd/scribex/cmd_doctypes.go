package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"scribex/pkg/doctypes"
)

func doctypesCmd(root string, a []string) error {
	if len(a) == 0 {
		return fmt.Errorf("doctypes list | show <id> | validate")
	}
	c, err := doctypes.Load(root)
	if err != nil && a[0] != "validate" {
		return err
	}
	switch a[0] {
	case "list":
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tENABLED\tGROUNDING\tAUDIENCE\tGRANULARITY\tOUTPUT")
		for _, t := range c.Types {
			fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%s\t%s\n", t.ID, t.Enabled, t.Grounding, t.Audience, t.Granularity, t.Output)
		}
		return tw.Flush()
	case "show":
		if len(a) != 2 {
			return fmt.Errorf("doctypes show <id>")
		}
		t, ok := c.Get(a[1])
		if !ok {
			return fmt.Errorf("unknown doc type %q", a[1])
		}
		b, _ := json.MarshalIndent(t, "", "  ")
		fmt.Println(string(b))
		return nil
	case "validate":
		// Load fails on structural problems; report those and value problems together.
		var probs []string
		if err != nil {
			probs = append(probs, strings.TrimPrefix(err.Error(), doctypes.FileName+": "))
		}
		probs = append(probs, doctypes.Validate(root, c)...)
		if len(probs) > 0 {
			return fmt.Errorf("%s", strings.Join(probs, "\n"))
		}
		fmt.Println("doctypes ok")
		return nil
	}
	return fmt.Errorf("unknown doctypes command %q", a[0])
}
