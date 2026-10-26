# Scribex Integrations

Scribex integrates with GitHub, Jira, and Confluence to cross-reference changes, post status comments, and sync technical documentation.

## HTTP Transport & Network Security

All network calls to integration endpoints (GitHub API, Jira REST API, Confluence REST API) go through the hardened `httpx` transport.

### Private Network Endpoints (On-Premise Deployments)
By default, the Scribex HTTP transport rejects connections to private IP ranges (RFC 1918 `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, CGNAT, and IPv6 ULA) to prevent Server-Side Request Forgery (SSRF).

When integrating with on-premise installations located on internal corporate subnets (such as **GitHub Enterprise Server**, **Jira Data Center**, or **Confluence Server/DC**), enable private IP target resolution by setting:

```bash
export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
```

> [!NOTE]
> Even with `SCRIBEX_ALLOW_PRIVATE_HOSTS=1` enabled, link-local and cloud instance metadata addresses (`169.254.169.254`, `169.254.0.0/16`, `fe80::/10`) remain strictly blocked to safeguard cloud credentials.

### Authentication & Secrets Scrubbing
Tokens passed via environment variables (such as `GITHUB_TOKEN`, `ATLASSIAN_TOKEN`) are treated as confidential secrets. Any error returned by the HTTP client automatically scrubs these tokens from error messages, status lines, and echoed response bodies.
