package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"artix/internal/forgesec"
	"artix/pkg/policy"
)

// GitHubClient interacts with GitHub's REST API.
type GitHubClient struct {
	auth       ForgeAuth
	httpClient *http.Client
	baseURL    string
}

// NewGitHubClient creates a GitHub API client.
func NewGitHubClient(auth ForgeAuth) *GitHubClient {
	baseURL := auth.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GitHubClient{
		auth: auth,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// CreatePullRequest opens a new Pull Request on GitHub.
func (c *GitHubClient) CreatePullRequest(target *RemoteRepoTarget, req *PullRequestRequest) (*PullRequestResponse, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls", c.baseURL, target.Owner, target.Repo)

	payload := map[string]string{
		"title": req.Title,
		"body":  req.Body,
		"head":  req.Head,
		"base":  req.Base,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pull request payload: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.auth.Token)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api error (status %d): check token and repo permissions", resp.StatusCode)
	}

	var ghResp struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghResp); err != nil {
		return nil, fmt.Errorf("failed to decode github response: %w", err)
	}

	return &PullRequestResponse{
		ID:      ghResp.ID,
		Number:  ghResp.Number,
		Title:   ghResp.Title,
		HTMLURL: ghResp.HTMLURL,
		URL:     ghResp.HTMLURL,
		State:   ghResp.State,
	}, nil
}

// VerifyPRApproval fetches PR and review details server-side from GitHub API.
func (c *GitHubClient) VerifyPRApproval(ctx context.Context, target *RemoteRepoTarget, prNumber int, targetCommitSHA string) (*policy.PRApproval, error) {
	if strings.TrimSpace(targetCommitSHA) == "" {
		return nil, errors.New("forge approval verification failed: targetCommitSHA cannot be empty")
	}

	// 1. Fetch PR details (Author and Head commit SHA)
	prURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, target.Owner, target.Repo, prNumber)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, prURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.auth.Token)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("forge api error fetching PR details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("forge api error fetching PR details: status %d", resp.StatusCode)
	}

	var prDetails struct {
		User struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prDetails); err != nil {
		return nil, fmt.Errorf("failed to decode PR response: %w", err)
	}

	headSHA := prDetails.Head.SHA
	if headSHA != targetCommitSHA {
		return nil, fmt.Errorf("%w: PR head commit %s differs from target commit %s (stale commit)", ErrStaleCommitMismatch, headSHA, targetCommitSHA)
	}

	// 2. Fetch all PR reviews with pagination
	type GHReview struct {
		ID   int64 `json:"id"`
		User struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		State    string `json:"state"`
		CommitID string `json:"commit_id"`
	}

	var allReviews []GHReview
	page := 1
	for {
		reviewsURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews?per_page=100&page=%d", c.baseURL, target.Owner, target.Repo, prNumber, page)
		rReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reviewsURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create http request: %w", err)
		}
		rReq.Header.Set("Authorization", "Bearer "+c.auth.Token)
		rReq.Header.Set("Accept", "application/vnd.github.v3+json")

		rResp, err := c.httpClient.Do(rReq)
		if err != nil {
			return nil, fmt.Errorf("%w: fetching reviews: %v", ErrForgeUnavailable, err)
		}

		if rResp.StatusCode < 200 || rResp.StatusCode >= 300 {
			rResp.Body.Close()
			return nil, fmt.Errorf("%w: status %d fetching reviews", ErrForgeUnavailable, rResp.StatusCode)
		}

		var pageReviews []GHReview
		if err := json.NewDecoder(rResp.Body).Decode(&pageReviews); err != nil {
			rResp.Body.Close()
			return nil, fmt.Errorf("failed to decode reviews response: %w", err)
		}
		rResp.Body.Close()

		if len(pageReviews) == 0 {
			break
		}
		allReviews = append(allReviews, pageReviews...)
		if len(pageReviews) < 100 {
			break
		}
		page++
	}

	if len(allReviews) == 0 {
		return nil, fmt.Errorf("%w: no reviews found on PR #%d", ErrNoReviewsYet, prNumber)
	}

	// Order matters: reviews are chronological. Track latest review state per reviewer.
	latestByUser := make(map[string]GHReview)
	for _, rev := range allReviews {
		latestByUser[strings.ToLower(rev.User.Login)] = rev
	}

	// Sort reviewers deterministically
	var logins []string
	for login := range latestByUser {
		logins = append(logins, login)
	}
	sort.Strings(logins)

	// Any CHANGES_REQUESTED
	for _, login := range logins {
		rev := latestByUser[login]
		if strings.EqualFold(rev.State, "CHANGES_REQUESTED") {
			isBot := strings.EqualFold(rev.User.Type, "Bot") ||
				strings.Contains(strings.ToLower(rev.User.Login), "[bot]") ||
				strings.HasSuffix(strings.ToLower(rev.User.Login), "-bot") ||
				strings.EqualFold(rev.User.Login, "artix-agent") ||
				strings.EqualFold(rev.User.Login, "artix-bot")
			if isBot {
				return nil, fmt.Errorf("%w: bot %q changes requested (unauthorized rejection)", ErrBotApprover, rev.User.Login)
			}
			pol := policy.Active()
			if len(pol.AllowedApprovers) > 0 {
				allowed := false
				for _, a := range pol.AllowedApprovers {
					if strings.EqualFold(a, rev.User.Login) {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, fmt.Errorf("%w: unauthorized reviewer %q changes requested", ErrUnauthorizedApprover, rev.User.Login)
				}
			}
			return nil, fmt.Errorf("%w: review changes requested by authorized reviewer %q", ErrChangesRequested, rev.User.Login)
		}
	}

	// Find an approved review deterministically
	for _, login := range logins {
		rev := latestByUser[login]
		if strings.EqualFold(rev.State, "APPROVED") {
			// Check rejections:
			// 1. Approver == PR author
			if strings.EqualFold(rev.User.Login, prDetails.User.Login) {
				return nil, fmt.Errorf("%w: PR author %q cannot approve their own PR", ErrAuthorSelfApproval, rev.User.Login)
			}
			// 2. Bot / App account
			if strings.EqualFold(rev.User.Type, "Bot") ||
				strings.Contains(strings.ToLower(rev.User.Login), "[bot]") ||
				strings.HasSuffix(strings.ToLower(rev.User.Login), "-bot") ||
				strings.EqualFold(rev.User.Login, "artix-agent") ||
				strings.EqualFold(rev.User.Login, "artix-bot") {
				return nil, fmt.Errorf("%w: bot or App account %q cannot approve PR", ErrBotApprover, rev.User.Login)
			}
			// 3. Stale commit on review
			if rev.CommitID != targetCommitSHA {
				return nil, fmt.Errorf("%w: approval on stale commit %s (current head is %s)", ErrStaleCommitMismatch, rev.CommitID, targetCommitSHA)
			}
			// 4. Allowed approvers in policy
			pol := policy.Active()
			if len(pol.AllowedApprovers) > 0 {
				allowed := false
				for _, a := range pol.AllowedApprovers {
					if strings.EqualFold(a, rev.User.Login) {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, fmt.Errorf("%w: approver %q is not authorized in allowedApprovers list %v", ErrUnauthorizedApprover, rev.User.Login, pol.AllowedApprovers)
				}
			}

			sig := forgesec.SignToken(rev.User.Login, prDetails.User.Login, "APPROVED", rev.CommitID, "github_api_server_verified")
			return &policy.PRApproval{
				ApproverUsername: rev.User.Login,
				AuthorUsername:   prDetails.User.Login,
				State:            "APPROVED",
				CommitSHA:        rev.CommitID,
				Signature:        sig,
				Source:           "github_api_server_verified",
				VerifiedByForge:  true,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: no valid human APPROVED review found", ErrNoReviewsYet)
}

// NewGitHubVerifier creates a LoopOptions.ForgeVerifier callback wired to the GitHub API client.
func NewGitHubVerifier(client *GitHubClient, target *RemoteRepoTarget, prNumber int) func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
	return func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return client.VerifyPRApproval(ctx, target, prNumber, commitSHA)
	}
}


