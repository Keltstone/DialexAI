package ci

import "testing"

func TestIsPullRequestEvent(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantPR    bool
		wantMatch string
	}{
		// GitHub positives
		{
			name:      "github_pull_request",
			env:       map[string]string{"GITHUB_EVENT_NAME": "pull_request"},
			wantPR:    true,
			wantMatch: "GITHUB_EVENT_NAME=pull_request",
		},
		{
			name:      "github_pull_request_target",
			env:       map[string]string{"GITHUB_EVENT_NAME": "pull_request_target"},
			wantPR:    true,
			wantMatch: "GITHUB_EVENT_NAME=pull_request_target",
		},
		{
			name:      "github_pull_request_review",
			env:       map[string]string{"GITHUB_EVENT_NAME": "pull_request_review"},
			wantPR:    true,
			wantMatch: "GITHUB_EVENT_NAME=pull_request_review",
		},
		// GitHub negatives
		{
			name:   "github_push",
			env:    map[string]string{"GITHUB_EVENT_NAME": "push"},
			wantPR: false,
		},
		{
			name:   "github_schedule",
			env:    map[string]string{"GITHUB_EVENT_NAME": "schedule"},
			wantPR: false,
		},
		{
			name:   "github_workflow_dispatch",
			env:    map[string]string{"GITHUB_EVENT_NAME": "workflow_dispatch"},
			wantPR: false,
		},

		// GitLab positives
		{
			name:      "gitlab_merge_request_event",
			env:       map[string]string{"CI_PIPELINE_SOURCE": "merge_request_event"},
			wantPR:    true,
			wantMatch: "CI_PIPELINE_SOURCE=merge_request_event",
		},
		{
			name:      "gitlab_external_pull_request_event",
			env:       map[string]string{"CI_PIPELINE_SOURCE": "external_pull_request_event"},
			wantPR:    true,
			wantMatch: "CI_PIPELINE_SOURCE=external_pull_request_event",
		},
		// GitLab negatives
		{
			name:   "gitlab_push",
			env:    map[string]string{"CI_PIPELINE_SOURCE": "push"},
			wantPR: false,
		},
		{
			name:   "gitlab_schedule",
			env:    map[string]string{"CI_PIPELINE_SOURCE": "schedule"},
			wantPR: false,
		},

		// Bitbucket positives & negatives
		{
			name:      "bitbucket_pr",
			env:       map[string]string{"BITBUCKET_PR_ID": "42"},
			wantPR:    true,
			wantMatch: "BITBUCKET_PR_ID=42",
		},
		{
			name:   "bitbucket_empty",
			env:    map[string]string{"BITBUCKET_PR_ID": ""},
			wantPR: false,
		},

		// Azure positives & negatives
		{
			name:      "azure_build_reason_pr",
			env:       map[string]string{"BUILD_REASON": "PullRequest"},
			wantPR:    true,
			wantMatch: "BUILD_REASON=PullRequest",
		},
		{
			name:   "azure_build_reason_manual",
			env:    map[string]string{"BUILD_REASON": "Manual"},
			wantPR: false,
		},
		{
			name:      "azure_system_pr_id",
			env:       map[string]string{"SYSTEM_PULLREQUEST_PULLREQUESTID": "1001"},
			wantPR:    true,
			wantMatch: "SYSTEM_PULLREQUEST_PULLREQUESTID=1001",
		},

		// Jenkins positives & negatives
		{
			name:      "jenkins_change_id",
			env:       map[string]string{"CHANGE_ID": "PR-12"},
			wantPR:    true,
			wantMatch: "CHANGE_ID=PR-12",
		},
		{
			name:   "jenkins_empty",
			env:    map[string]string{"CHANGE_ID": ""},
			wantPR: false,
		},

		// Travis positives & negatives
		{
			name:      "travis_pr_num",
			env:       map[string]string{"TRAVIS_PULL_REQUEST": "123"},
			wantPR:    true,
			wantMatch: "TRAVIS_PULL_REQUEST=123",
		},
		{
			name:      "travis_pr_true",
			env:       map[string]string{"TRAVIS_PULL_REQUEST": "true"},
			wantPR:    true,
			wantMatch: "TRAVIS_PULL_REQUEST=true",
		},
		{
			name:   "travis_pr_false",
			env:    map[string]string{"TRAVIS_PULL_REQUEST": "false"},
			wantPR: false,
		},
		{
			name:   "travis_empty",
			env:    map[string]string{"TRAVIS_PULL_REQUEST": ""},
			wantPR: false,
		},

		// Empty environment
		{
			name:   "empty_env",
			env:    map[string]string{},
			wantPR: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPR, gotMatch := IsPullRequestEvent(func(k string) string { return tc.env[k] })
			if gotPR != tc.wantPR {
				t.Fatalf("expected isPR=%v, got %v", tc.wantPR, gotPR)
			}
			if tc.wantPR && gotMatch != tc.wantMatch {
				t.Errorf("expected match=%q, got %q", tc.wantMatch, gotMatch)
			}
		})
	}
}
