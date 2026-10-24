// Package github posts the Scribex report as a single, updated-in-place PR comment.
package github

import (
	"fmt"
	"strings"

	"scribex/pkg/httpx"
)

const marker = "<!-- scribex-report -->"

// Upsert creates or updates the Scribex comment on a PR.
func Upsert(api, token, repo string, pr int, body string) error {
	h := map[string]string{"Authorization": "Bearer " + token}
	base := fmt.Sprintf("%s/repos/%s", strings.TrimRight(api, "/"), repo)
	var existing []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	if err := httpx.Do("GET", fmt.Sprintf("%s/issues/%d/comments?per_page=100", base, pr), h, nil, &existing); err != nil {
		return err
	}
	payload := map[string]string{"body": marker + "\n" + body}
	for _, c := range existing {
		if strings.HasPrefix(c.Body, marker) {
			return httpx.Do("PATCH", fmt.Sprintf("%s/issues/comments/%d", base, c.ID), h, payload, nil)
		}
	}
	return httpx.Do("POST", fmt.Sprintf("%s/issues/%d/comments", base, pr), h, payload, nil)
}
