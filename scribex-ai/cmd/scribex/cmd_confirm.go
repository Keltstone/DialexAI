package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"scribex/pkg/derived"
	"scribex/pkg/fsx"
	"scribex/pkg/safepath"
)

// confirmCmd re-stamps a derived section after a human has reviewed it: recorded source hashes become
// the current ones, the "AI draft" banner is dropped and confirmed=<date> is recorded.
func confirmCmd(root string, a []string) error {
	if len(a) != 1 || !strings.Contains(a[0], "#") {
		return fmt.Errorf("confirm <doc.md>#<typeID:unit>")
	}
	doc, id, _ := strings.Cut(a[0], "#")
	p, err := safepath.Join(root, doc)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	out, _, err := derived.Confirm(root, string(b), id, time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		return err
	}
	if err := fsx.WriteFileIn(root, doc, []byte(out), 0o644); err != nil {
		return err
	}
	fmt.Println("confirmed", id, "in", doc)
	return nil
}
