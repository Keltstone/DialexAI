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

## `.scribex/` Directory Guard

The `.scribex` directory stores local project configuration (`config.yaml`), local secrets (`secrets.env`), manifest definitions (`manifest.yaml`), and pulled Confluence pages (`pages/`).

To prevent path traversal and symlink redirection attacks:
- `.scribex` **must be a real directory** located directly inside the repository root.
- If `.scribex` is a symlink (even to an internal directory), commands accessing it (`init`, `secrets set`, `config set`, `confluence pull/push`) immediately fail closed with:
  ```
  .scribex must be a real directory inside the repository
  ```
- All secret and page writes are performed atomically (`0o600` for secrets, `0o644` for state/pages).

## Confluence Integration

Scribex pulls page bodies from Confluence and pushes draft revisions based on code signatures.

### Manifest Format & Validation Rules

Confluence mapping is defined in `.scribex/manifest.yaml`. Each entry maps a Confluence page to code symbols:

```yaml
- page: "123456789"
  space: "ENG"
  title: "Authentication API Specification"
  symbols:
    - "pkg/auth/jwt.go#VerifyToken"
    - "pkg/auth/session.go#NewSession"
```

Every manifest entry is validated upon loading:
1. **`page`**: Must contain digits only (1 to 20 digits, matching `safepath.ValidPageID`). Directory traversal sequences (`..`), non-digits, or empty page IDs are strictly rejected. Page IDs must be unique across entries.
2. **`space`**: Must match `^[A-Za-z0-9~][A-Za-z0-9_~-]{0,254}$`.
3. **`title`**: Non-empty string, maximum 255 runes, containing no control characters or newlines.
4. **`symbols`**: Each symbol item must be in the format `<relpath>#<Sym>` with exactly one `#`. The `<relpath>` portion must be a clean relative path (no `..`, no absolute paths, no NUL bytes, max 4096 bytes).

### Draft-Only & Idempotent Push

- **Draft-only guarantee**: `scribex confluence push` only creates drafts (`"status": "draft"`) for lead review. It never directly overwrites or publishes live page versions.
- **Push idempotence**: Scribex records SHA-256 hashes of the generated page content in `.scribex/state.json`. If a page's content has not changed since the last push, `push` skips re-creating the draft.
- **Corrupt state handling**: A corrupt `state.json` causes `push` to fail immediately rather than silently overwriting state.
- **Body size cap**: Confluence draft bodies are capped at 1 MiB to prevent denial-of-service from oversized documentation.

## Jira Integration

Scribex compares contract changes between git branches against Jira ticket specifications.

### Jira Issue Key Format

All Jira issue keys passed via the `-key` flag, extracted from branch names, or processed by API clients must strictly conform to:
```
^[A-Z][A-Z0-9_]{1,30}-[1-9][0-9]{0,9}$
```
- Examples of valid keys: `PROJ-1`, `TEAM_A-1234`.
- Lowercase keys (`proj-1`), paths (`PROJ-1/../../x`), query injections (`PROJ-1?x=1`), or issue numbers starting with zero (`PROJ-0`) are rejected before any network request is made.

### `jira -post` Idempotence Marker

When using `scribex jira -post`, Scribex reality-checks PR deltas against the ticket summary and description:
- Before posting a comment, Scribex inspects existing comments (paginating up to 5 pages of 100 comments each) for a hidden marker:
  ```html
  <!-- scribex-reality-check:<sha256-hash> -->
  ```
- If a comment containing the matching content hash already exists, the post is skipped idempotently.
- When posting a new comment, Scribex appends this hidden marker so subsequent runs will not duplicate comments.

## GitHub Integration

Scribex posts automated PR comments summarizing anchor verification status and contract changes.

- **Repository validation**: `-repo` must conform to `^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$` (neither part may be `.` or `..`).
- **PR bounds**: `-pr` must be a positive integer in the range `1` to `2147483647` (`2^31 - 1`).
- **Base URL cleaning**: `GITHUB_API_URL` and `ATLASSIAN_URL` must have scheme `https` (or `http` loopback for local testing), and cannot contain userinfo, queries, or URL fragments.
