package engines

import (
	"math"
	"regexp"
)

// GitleaksRulesVersion is the vendored gitleaks rule set version.
const GitleaksRulesVersion = "8.21.2"

type SecretRule struct {
	ID          string
	Description string
	Regex       *regexp.Regexp
}

var gitleaksRules = []SecretRule{
	{"stripe-access-token", "Stripe API key", regexp.MustCompile(`(?:sk|rk)_live_[0-9a-zA-Z]{24,}`)},
	{"slack-token", "Slack token", regexp.MustCompile(`xox[baprs]-[0-9a-zA-Z]{10,48}`)},
	{"slack-app-token", "Slack App-level token", regexp.MustCompile(`xapp-[0-9]-[A-Z0-9]+-[0-9]+-[a-zA-Z0-9]+`)},
	{"google-api-key", "Google API key", regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)},
	{"azure-storage-account-key", "Azure Storage Account Key", regexp.MustCompile(`AccountKey=[A-Za-z0-9+/=]{60,}`)},
	{"npm-access-token", "npm access token", regexp.MustCompile(`npm_[A-Za-z0-9]{36}`)},
	{"aws-access-token", "AWS Access Key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"aws-secret-key", "AWS Secret Key", regexp.MustCompile(`(?i)(?:aws_secret_access_key|aws_secret|secret_key)\s*[:=]\s*["']?([A-Za-z0-9/+=]{40})["']?`)},
	{"github-pat", "GitHub Personal Access Token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{36,}`)},
	{"github-fine-grained-pat", "GitHub Fine-Grained PAT", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{82}`)},
	{"gitlab-pat", "GitLab Personal Access Token", regexp.MustCompile(`glpat-[0-9a-zA-Z\-_]{20,}`)},
	{"gitlab-pipeline-token", "GitLab Pipeline Trigger Token", regexp.MustCompile(`glcbt-[0-9a-zA-Z\-_]{20,}`)},
	{"openai-api-key", "OpenAI API Key", regexp.MustCompile(`sk-(?:proj-)?[a-zA-Z0-9_\-]{20,}`)},
	{"pypi-upload-token", "PyPI upload token", regexp.MustCompile(`pypi-AgEIcHlwaS5vcmc[A-Za-z0-9\-_]{50,}`)},
	{"twilio-api-key", "Twilio API Key", regexp.MustCompile(`SK[0-9a-fA-F]{32}`)},
	{"sendgrid-api-token", "SendGrid API Key", regexp.MustCompile(`SG\.[0-9a-zA-Z\-_]{22}\.[0-9a-zA-Z\-_]{43}`)},
	{"mailgun-api-key", "Mailgun API Key", regexp.MustCompile(`key-[0-9a-zA-Z]{32}`)},
	{"telegram-bot-token", "Telegram Bot Token", regexp.MustCompile(`[0-9]{8,10}:[a-zA-Z0-9_\-]{35}`)},
	{"shopify-access-token", "Shopify Access Token", regexp.MustCompile(`shpat_[0-9a-fA-F]{32}`)},
	{"shopify-custom-app-token", "Shopify Custom App Token", regexp.MustCompile(`shpca_[0-9a-fA-F]{32}`)},
	{"shopify-shared-secret", "Shopify Shared Secret", regexp.MustCompile(`shpss_[0-9a-fA-F]{32}`)},
	{"square-access-token", "Square Access Token", regexp.MustCompile(`sq0atp-[0-9A-Za-z\-_]{22,}`)},
	{"square-oauth-secret", "Square OAuth Secret", regexp.MustCompile(`sq0csp-[0-9A-Za-z\-_]{43}`)},
	{"vault-token", "HashiCorp Vault Token", regexp.MustCompile(`hvs\.[a-zA-Z0-9_\-]{24,}`)},
	{"discord-bot-token", "Discord Bot Token", regexp.MustCompile(`[A-Za-z0-9_\-]{24,28}\.[A-Za-z0-9_\-]{6}\.[A-Za-z0-9_\-]{27,38}`)},
	{"datadog-api-key", "Datadog API Key", regexp.MustCompile(`ddp_[0-9a-fA-F]{32}`)},
	{"heroku-api-key", "Heroku API Key", regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)},
	{"digitalocean-token", "DigitalOcean Token", regexp.MustCompile(`dop_v1_[0-9a-fA-F]{64}`)},
	{"grafana-api-key", "Grafana Service Account Token", regexp.MustCompile(`glsa_[0-9a-zA-Z]{32}_[0-9a-fA-F]{8}`)},
	{"postman-api-key", "Postman API Key", regexp.MustCompile(`PMAK-[0-9a-fA-F]{24}-[0-9a-fA-F]{34}`)},
	{"sentry-auth-token", "Sentry Auth Token", regexp.MustCompile(`sntrys_[0-9a-zA-Z]{64}`)},
	{"mapbox-api-token", "Mapbox API Token", regexp.MustCompile(`pk\.[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+`)},
	{"coinbase-api-key", "Coinbase API Key", regexp.MustCompile(`organizations/[0-9a-f\-]{36}/apiKeys/[0-9a-f\-]{36}`)},
	{"algolia-api-key", "Algolia API Key", regexp.MustCompile(`latency-[0-9a-fA-F]{32}`)},
	{"asana-pat", "Asana PAT", regexp.MustCompile(`0/[0-9]{16}/[0-9]{16}:[0-9a-fA-F]{32}`)},
	{"bitbucket-app-password", "Bitbucket App Password", regexp.MustCompile(`ATBB[0-9a-zA-Z]{32,}`)},
	{"circleci-token", "CircleCI Personal Access Token", regexp.MustCompile(`circleci_pat_[0-9a-zA-Z]{40,}`)},
	{"doppler-token", "Doppler Token", regexp.MustCompile(`dp\.pt\.[0-9a-zA-Z]{40,}`)},
	{"dynatrace-token", "Dynatrace API Token", regexp.MustCompile(`dt0c01\.[0-9a-zA-Z]{12}\.[0-9a-zA-Z]{64}`)},
	{"fastly-api-token", "Fastly API Token", regexp.MustCompile(`FastlyAPIKey-[0-9a-zA-Z]{32,}`)},
	{"figma-pat", "Figma Personal Access Token", regexp.MustCompile(`figd_[0-9a-zA-Z_\-]{40,}`)},
	{"finnhub-token", "Finnhub Token", regexp.MustCompile(`sandbox_c[0-9a-zA-Z]{18,}`)},
	{"flutterwave-secret-key", "Flutterwave Secret Key", regexp.MustCompile(`FLWSECK-[0-9a-fA-F]{32,}-X`)},
	{"intercom-access-token", "Intercom Access Token", regexp.MustCompile(`dG9rZW46[0-9a-zA-Z=]{20,}`)},
	{"launchdarkly-sdk-key", "LaunchDarkly SDK Key", regexp.MustCompile(`sdk-[0-9a-f\-]{36}`)},
	{"linear-api-key", "Linear API Key", regexp.MustCompile(`lin_api_[0-9a-zA-Z]{40,}`)},
	{"messagebird-api-key", "MessageBird API Key", regexp.MustCompile(`live_[0-9a-zA-Z]{24,}`)},
	{"new-relic-user-key", "New Relic User Key", regexp.MustCompile(`NRAK-[0-9A-Z]{27}`)},
	{"pagerduty-api-key", "PagerDuty API Key", regexp.MustCompile(`y_[0-9a-zA-Z]{32}`)},
	{"private-key", "Private Key", regexp.MustCompile(`-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----`)},
	{"secret.db-url", "Database URL", regexp.MustCompile(`(?i)[a-z0-9+.-]+://[^:\s]+:[^@\s]+@[^/\s]+/\S*`)},
	{"secret.generic-assignment", "Generic credential assignment", regexp.MustCompile(`(?i)(api[_-]?key|secret|passwd|password|token)\s*[:=]\s*["']([^"'\r\n]{16,})["']`)},
	{"secret.high-entropy", "High-entropy token", regexp.MustCompile(`(?i)(bearer\s+[A-Za-z0-9_\-\.]{24,}|eyJ[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}|(?:entropy)\s*[:=]\s*["'][A-Za-z0-9+/=_\-\.]{20,}["'])`)},
}

// ShannonEntropy calculates the Shannon entropy of a string.
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]float64)
	for _, r := range s {
		freq[r]++
	}
	var entropy float64
	l := float64(len(s))
	for _, count := range freq {
		p := count / l
		entropy -= p * math.Log2(p)
	}
	return entropy
}
