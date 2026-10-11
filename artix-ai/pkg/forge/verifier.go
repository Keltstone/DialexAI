package forge

import (
	"context"
	"os"
	"strings"

	"artix/pkg/policy"
)

// ProductionVerifierConfig holds configuration for initializing a production forge verifier.
type ProductionVerifierConfig struct {
	ForgeType string // "github" or "gitlab"
	BaseURL   string
	Token     string
	Owner     string
	Repo      string
	PRNumber  int
	MRIID     int
}

// NewProductionVerifier creates a production-grade ForgeVerifier for autonomous mode.
func NewProductionVerifier(cfg ProductionVerifierConfig) func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
	forgeType := strings.ToLower(strings.TrimSpace(cfg.ForgeType))
	if forgeType == "" {
		if os.Getenv("GITLAB_CI") != "" || os.Getenv("CI_MERGE_REQUEST_IID") != "" {
			forgeType = "gitlab"
		} else {
			forgeType = "github"
		}
	}

	token := cfg.Token
	if token == "" {
		if forgeType == "gitlab" {
			token = os.Getenv("GITLAB_TOKEN")
			if token == "" {
				token = os.Getenv("CI_JOB_TOKEN")
			}
		} else {
			token = os.Getenv("GITHUB_TOKEN")
			if token == "" {
				token = os.Getenv("GH_TOKEN")
			}
		}
	}

	target := &RemoteRepoTarget{
		Owner: cfg.Owner,
		Repo:  cfg.Repo,
	}

	prNum := cfg.PRNumber
	if prNum <= 0 && cfg.MRIID > 0 {
		prNum = cfg.MRIID
	}

	if forgeType == "gitlab" {
		glClient := NewGitLabClient(ForgeAuth{
			Type:    ForgeGitLab,
			Token:   token,
			BaseURL: cfg.BaseURL,
		})
		return NewGitLabVerifier(glClient, target, prNum)
	}

	ghClient := NewGitHubClient(ForgeAuth{
		Type:    ForgeGitHub,
		Token:   token,
		BaseURL: cfg.BaseURL,
	})
	return NewGitHubVerifier(ghClient, target, prNum)
}

// BuildVerifierFromEnvironment attempts to infer forge parameters from CI/Git environment.
func BuildVerifierFromEnvironment(owner, repo string, prNumber int) func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
	forgeType := os.Getenv("ARTIX_FORGE_TYPE")
	if forgeType == "" {
		if os.Getenv("GITLAB_CI") != "" {
			forgeType = "gitlab"
		} else {
			forgeType = "github"
		}
	}

	token := os.Getenv("ARTIX_FORGE_TOKEN")
	baseURL := os.Getenv("ARTIX_FORGE_URL")

	return NewProductionVerifier(ProductionVerifierConfig{
		ForgeType: forgeType,
		BaseURL:   baseURL,
		Token:     token,
		Owner:     owner,
		Repo:      repo,
		PRNumber:  prNumber,
	})
}
