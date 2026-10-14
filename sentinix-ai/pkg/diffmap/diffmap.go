// Package diffmap turns a unified diff into attack-surface items.
package diffmap

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

type Item struct {
	Type   string `json:"type"`   // endpoint, permission, dependency
	Change string `json:"change"` // added, removed
	File   string `json:"file"`
	Detail string `json:"detail"`
}

var (
	endpoint   = regexp.MustCompile(`\b(get|post|put|delete|patch)\s*\(\s*"(/[^"]*)"|@(Get|Post|Put|Delete|Patch)Mapping`)
	permission = regexp.MustCompile(`<uses-permission[^>]*android:name="([^"]+)"|android:exported="true"`)
	depFile    = regexp.MustCompile(`(^|/)(build\.gradle(\.kts)?|libs\.versions\.toml|package\.json|go\.mod|pom\.xml|requirements\.txt)$`)
)

func Parse(r io.Reader) ([]Item, error) {
	var items []Item
	file := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "+++ b/"):
			file = l[6:]
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case len(l) > 1 && (l[0] == '+' || l[0] == '-'):
			change := map[byte]string{'+': "added", '-': "removed"}[l[0]]
			body := l[1:]
			add := func(t string) {
				items = append(items, Item{t, change, file, strings.TrimSpace(body)})
			}
			switch {
			case endpoint.MatchString(body):
				add("endpoint")
			case permission.MatchString(body):
				add("permission")
			case depFile.MatchString(file) && strings.TrimSpace(body) != "":
				add("dependency")
			}
		}
	}
	return items, sc.Err()
}
