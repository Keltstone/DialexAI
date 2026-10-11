package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"artix/internal/forgesec"
	"artix/pkg/policy"
)

// GitLabClient interacts with GitLab's REST API.
type GitLabClient struct {
	auth       ForgeAuth
	httpClient *http.Client
	baseURL    string
}

// NewGitLabClient creates a GitLab API client.
func NewGitLabClient(auth ForgeAuth) *GitLabClient {
	baseURL := auth.BaseURL
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GitLabClient{
		auth: auth,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// CreatePullRequest opens a new Merge Request on GitLab.
func (c *GitLabClient) CreatePullRequest(target *RemoteRepoTarget, req *PullRequestRequest) (*PullRequestResponse, error) {
	projectPath := fmt.Sprintf("%s/%s", target.Owner, target.Repo)
	escapedPath := url.PathEscape(projectPath)
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests", c.baseURL, escapedPath)

	payload := map[string]string{
		"title":         req.Title,
		"description":   req.Body,
		"source_branch": req.Head,
		"target_branch": req.Base,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merge request payload: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("PRIVATE-TOKEN", c.auth.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gitlab api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab api error (status %d): check token and project permissions", resp.StatusCode)
	}

	var glResp struct {
		ID     int64  `json:"id"`
		IID    int    `json:"iid"`
		Title  string `json:"title"`
		WebURL string `json:"web_url"`
		State  string `json:"state"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&glResp); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab response: %w", err)
	}

	return &PullRequestResponse{
		ID:      glResp.ID,
		Number:  glResp.IID,
		Title:   glResp.Title,
		HTMLURL: glResp.WebURL,
		URL:     glResp.WebURL,
		State:   glResp.State,
	}, nil
}

// VerifyMRApproval fetches Merge Request and approval details server-side from GitLab API.
func (c *GitLabClient) VerifyMRApproval(ctx context.Context, target *RemoteRepoTarget, mrIID int, targetCommitSHA string) (*policy.PRApproval, error) {
	if strings.TrimSpace(targetCommitSHA) == "" {
		return nil, errors.New("forge approval verification failed: targetCommitSHA cannot be empty")
	}

	projectPath := fmt.Sprintf("%s/%s", target.Owner, target.Repo)
	escapedPath := url.PathEscape(projectPath)

	// 1. Fetch MR details
	mrURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d", c.baseURL, escapedPath, mrIID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mrURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.auth.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forge api error fetching MR details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("forge api error fetching MR details: status %d", resp.StatusCode)
	}

	var mrDetails struct {
		ID     int64  `json:"id"`
		IID    int    `json:"iid"`
		SHA    string `json:"sha"`
		State  string `json:"state"`
		Author struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mrDetails); err != nil {
		return nil, fmt.Errorf("failed to decode MR response: %w", err)
	}

	if mrDetails.SHA != targetCommitSHA {
		return nil, fmt.Errorf("%w: MR head commit %s differs from target commit %s (stale commit)", ErrStaleCommitMismatch, mrDetails.SHA, targetCommitSHA)
	}

	// 2. Fetch MR approvals
	approvalsURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/approvals", c.baseURL, escapedPath, mrIID)
	appReq, err := http.NewRequestWithContext(ctx, http.MethodGet, approvalsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	appReq.Header.Set("PRIVATE-TOKEN", c.auth.Token)

	appResp, err := c.httpClient.Do(appReq)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching MR approvals: %v", ErrForgeUnavailable, err)
	}
	defer appResp.Body.Close()

	if appResp.StatusCode < 200 || appResp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d fetching MR approvals", ErrForgeUnavailable, appResp.StatusCode)
	}

	var approvalsResp struct {
		ApprovedBy []struct {
			User struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
				Name     string `json:"name"`
			} `json:"user"`
		} `json:"approved_by"`
	}
	if err := json.NewDecoder(appResp.Body).Decode(&approvalsResp); err != nil {
		return nil, fmt.Errorf("failed to decode approvals response: %w", err)
	}

	if len(approvalsResp.ApprovedBy) == 0 {
		return nil, fmt.Errorf("%w: no approvals found on MR !%d", ErrNoReviewsYet, mrIID)
	}

	type approverInfo struct {
		username string
	}
	var approvers []approverInfo
	for _, ab := range approvalsResp.ApprovedBy {
		approvers = append(approvers, approverInfo{username: ab.User.Username})
	}
	sort.Slice(approvers, func(i, j int) bool {
		return approvers[i].username < approvers[j].username
	})

	for _, a := range approvers {
		u := a.username
		// Approver == author check
		if strings.EqualFold(u, mrDetails.Author.Username) {
			return nil, fmt.Errorf("%w: MR author %q cannot approve their own MR", ErrAuthorSelfApproval, u)
		}
		// Bot check
		uLower := strings.ToLower(u)
		if strings.Contains(uLower, "[bot]") ||
			strings.HasSuffix(uLower, "-bot") ||
			uLower == "artix-agent" ||
			uLower == "artix-bot" ||
			uLower == "bot" {
			return nil, fmt.Errorf("%w: bot or App account %q cannot approve MR", ErrBotApprover, u)
		}
		// Allowed approvers check
		pol := policy.Active()
		if len(pol.AllowedApprovers) > 0 {
			allowed := false
			for _, allowedUser := range pol.AllowedApprovers {
				if strings.EqualFold(allowedUser, u) {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, fmt.Errorf("%w: approver %q is not authorized in allowedApprovers list %v", ErrUnauthorizedApprover, u, pol.AllowedApprovers)
			}
		}

		sig := forgesec.SignToken(u, mrDetails.Author.Username, "APPROVED", targetCommitSHA, "gitlab_api_server_verified")
		return &policy.PRApproval{
			ApproverUsername: u,
			AuthorUsername:   mrDetails.Author.Username,
			State:            "APPROVED",
			CommitSHA:        targetCommitSHA,
			Signature:        sig,
			Source:           "gitlab_api_server_verified",
			VerifiedByForge:  true,
		}, nil
	}

	return nil, fmt.Errorf("%w: no valid human APPROVED review found", ErrNoReviewsYet)
}

// NewGitLabVerifier creates a LoopOptions.ForgeVerifier callback wired to the GitLab API client.
func NewGitLabVerifier(client *GitLabClient, target *RemoteRepoTarget, mrIID int) func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
	return func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return client.VerifyMRApproval(ctx, target, mrIID, commitSHA)
	}
}
