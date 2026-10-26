// Package ci detects whether the current execution is running inside an untrusted CI pull-request event.
package ci

import "strings"

// IsPullRequestEvent reports whether the environment indicates an untrusted pull-request context.
// Returns true and the name of the environment variable that triggered the refusal.
func IsPullRequestEvent(env func(string) string) (bool, string) {
	if env == nil {
		return false, ""
	}

	// 1. GitHub Actions
	if val := env("GITHUB_EVENT_NAME"); strings.HasPrefix(val, "pull_request") {
		return true, "GITHUB_EVENT_NAME=" + val
	}

	// 2. GitLab CI
	if val := env("CI_PIPELINE_SOURCE"); val == "merge_request_event" || val == "external_pull_request_event" {
		return true, "CI_PIPELINE_SOURCE=" + val
	}

	// 3. Bitbucket Pipelines
	if val := env("BITBUCKET_PR_ID"); val != "" {
		return true, "BITBUCKET_PR_ID=" + val
	}

	// 4. Azure Pipelines
	if val := env("BUILD_REASON"); val == "PullRequest" {
		return true, "BUILD_REASON=" + val
	}
	if val := env("SYSTEM_PULLREQUEST_PULLREQUESTID"); val != "" {
		return true, "SYSTEM_PULLREQUEST_PULLREQUESTID=" + val
	}

	// 5. Jenkins multibranch
	if val := env("CHANGE_ID"); val != "" {
		return true, "CHANGE_ID=" + val
	}

	// 6. Travis CI
	if val := env("TRAVIS_PULL_REQUEST"); val != "" && val != "false" {
		return true, "TRAVIS_PULL_REQUEST=" + val
	}

	return false, ""
}
