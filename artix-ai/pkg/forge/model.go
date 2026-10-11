package forge

import "errors"

// Typed forge errors for deterministic error handling and safe branch cleanup
var (
	ErrChangesRequested     = errors.New("forge approval verification failed: review changes requested")
	ErrReviewDismissed      = errors.New("forge approval verification failed: review dismissed")
	ErrStaleCommitMismatch  = errors.New("forge approval verification failed: stale commit mismatch")
	ErrAuthorSelfApproval   = errors.New("forge approval verification failed: author cannot approve own PR/MR")
	ErrUnauthorizedApprover = errors.New("forge approval verification failed: approver not authorized in allowedApprovers")
	ErrBotApprover          = errors.New("forge approval verification failed: bot or app account cannot approve")
	ErrForgedApproval       = errors.New("separation of duties violation: unverified or forged approval token")
	ErrNoReviewsYet         = errors.New("forge approval verification failed: no reviews submitted yet")
	ErrForgeUnavailable     = errors.New("forge service unavailable")
)

// ForgeType identifies the remote git hosting provider.
type ForgeType string

const (
	ForgeGitHub ForgeType = "github"
	ForgeGitLab ForgeType = "gitlab"
)

// ForgeAuth provides authentication credentials for a remote forge.
type ForgeAuth struct {
	Type     ForgeType `json:"type"`
	Token    string    `json:"token"`
	Username string    `json:"username,omitempty"`
	BaseURL  string    `json:"baseUrl,omitempty"` // For GitHub Enterprise / Self-hosted GitLab
}

// RemoteRepoTarget identifies a remote repository to operate against.
type RemoteRepoTarget struct {
	CloneURL    string `json:"cloneUrl"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"` // Base branch (e.g. main / master)
	PRNumber    int    `json:"prNumber,omitempty"`
	IssueNumber int    `json:"issueNumber,omitempty"`
}

// PullRequestRequest defines parameters to open a Pull Request / Merge Request.
type PullRequestRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"` // The newly created feature branch
	Base  string `json:"base"` // The target branch (e.g. main)
}

// PullRequestResponse contains the created PR details.
type PullRequestResponse struct {
	ID      int64  `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	HTMLURL string `json:"htmlUrl"`
	State   string `json:"state"`
}

// ForgeClient is the unified interface for remote Git hosts.
type ForgeClient interface {
	CreatePullRequest(target *RemoteRepoTarget, req *PullRequestRequest) (*PullRequestResponse, error)
}
