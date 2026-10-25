# sentinix-hunt-config (Hunt v2 Configuration & Ceilings)

Goal: user-configurable audit engine settings across file, environment, flags, and server DB.

Precedence (lowest to highest):
1. Built-in defaults: advisory gate, local-only, 200 calls, tools disabled, PoC disabled, no network.
2. Org/server defaults: enforced policy ceilings (max calls, max cost, allowed models, allowed tools).
3. Project configuration: `.sentinix/hunt.json` (strict JSON decoding, unknown keys rejected).
4. Environment variables: `SENTINIX_HUNT_*`.
5. CLI flags: `--ai`, `--policy`, `--max-calls`, `--taskflow`, `--tools`, `--gate`, `--dry-run`, etc.

Secret Discipline:
- Secrets allowed ONLY via `${ENV_VAR}` references (validated with regex `^\$\{([A-Z_][A-Z0-9_]*)\}$`).
- Literal tokens, passwords, API keys in any field cause strict validation failure (`Validate() error`).

Ceilings & Approvals:
- Org ceilings cap: max_calls <= org.MaxCalls, max_cost_usd <= org.MaxCostUSD, tools in org.AllowedTools.
- Loosening safeguards (raising to blocking/confirmed-only, local->cloud, raising budget) requires 2-person approval.
- Every config mutation is recorded in the immutable audit chain.

CLI Commands:
- `sentinix hunt config init`: writes safe template `.sentinix/hunt.json`.
- `sentinix hunt config show [--explain]`: outputs effective merged config and source attribution.
- `sentinix hunt config validate`: verifies path-level syntax, types, unknown keys, secrets, and ceilings.

Where: sentinix-ai/pkg/hunt/config.go, cmd/sentinix.
