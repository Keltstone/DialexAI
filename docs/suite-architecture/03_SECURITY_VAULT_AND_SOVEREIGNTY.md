# Dialex Suite: Sovereign Vault & Zero-Cloud Security

**Author:** Kolta Labs Security Architecture & Governance Team  
**Audience:** CISOs, Security Engineers, Compliance Officers, DevSecOps Leads  
**Scope:** Cryptographic algorithms, credential vaults, execution sandboxes, data isolation guarantees, and compliance certifications.

---

## 1. Threat Model & Security Posture

Dialex Suite is built for organizations where code, architectural strategy, and proprietary data **cannot be exposed to multi-tenant cloud providers**.

```
                           TRUST BOUNDARY
 ┌──────────────────────────────────────────────────────────────┐
 │                    LOCAL SOVEREIGN HOST                      │
 │                                                              │
 │   ┌──────────────────┐            ┌──────────────────────┐   │
 │   │  OS Secure Enclave│            │   Encrypted DB &     │   │
 │   │  (Keychain / HSM)│            │   Argon2id Vault     │   │
 │   └─────────┬────────┘            └──────────┬───────────┘   │
 │             │ Master Key                     │ Encrypted     │
 │             ▼                                ▼ Records       │
 │   ┌──────────────────────────────────────────────────────┐   │
 │   │           Dialex Suite Engine Daemons (Go / KMP)     │   │
 │   │   • AES-256-GCM Encrypted Keyring                    │   │
 │   │   • Process Memory Auto-Wipe (0-Filled on Exit)       │   │
 │   │   • Tamper-Evident Hash-Chained Audit Ledger         │   │
 │   └──────────────────────────┬───────────────────────────┘   │
 │                              │                               │
 │                              ▼                               │
 │   ┌──────────────────────────────────────────────────────┐   │
 │   │          OS-Level Execution Isolation Sandbox        │   │
 │   │   • macOS sandbox-exec / Linux Bubblewrap (bwrap)    │   │
 │   │   • Network Denied (--offline) by default            │   │
 │   │   • Ephemeral Shadow Git Worktrees                   │   │
 │   └──────────────────────────────────────────────────────┘   │
 └──────────────────────────────┼───────────────────────────────┘
                                │ Strictly user-consented egress
                                ▼
                   ┌─────────────────────────┐
                   │ Upstream AI Providers   │
                   │ (Anthropic, OpenAI, etc)│
                   └─────────────────────────┘
```

---

## 2. Cryptographic Storage & Credential Vault

All sensitive user credentials (API keys, OAuth tokens, session cookies, database credentials) are guarded by multi-tier cryptographic primitives.

### Tier 1: Hardware-Backed Master Key Derivation
- **macOS**: System Keychain Services (`kSecClassGenericPassword`), storing an AES-256 wrapping key guarded by device biometrics / login credentials.
- **Android**: Hardware Security Module (HSM) using Android Keystore System (`KeyGenParameterSpec.Builder(KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)`), using `StrongBox` or `TrustZone (TEE)`.
- **Linux**: FreeDesktop.org Secret Service API (`secret-tool`) via D-Bus.
- **Headless / Air-Gapped Fallback**: Keyfile on disk with strict POSIX mode `0600` (`-rw-------`), derived using **Argon2id** (memory: 64 MB, iterations: 3, parallelism: 4).

### Tier 2: Authenticated Payload Encryption (`AES-256-GCM`)
All credential payloads are encrypted using **AES-256 in Galois/Counter Mode (GCM)**, ensuring confidentiality and integrity:
$$\text{Ciphertext} = \text{AES-GCM-Encrypt}(K_{\text{derived}}, \text{Nonce}_{96}, \text{Plaintext}, \text{AAD})$$
- Nonces are 96-bit cryptographically secure pseudorandom numbers generated via `crypto/rand`.
- Authentication tags (128-bit) are validated prior to decryption; any bit tampering results in immediate cryptographic rejection (`crypto/cipher: message authentication failed`).

### Tier 3: In-Memory Auto-Wipe
When sessions terminate or tokens are rotated, all in-memory byte slices holding secrets are explicitly overwritten with zeroes using `crypto/subtle` before being returned to the garbage collector.

---

## 3. Sandboxed Execution Isolation

Both `artix-ai` and `kritix-ai` execute external tools, compilers, test suites, and browser engines. They enforce strict host containment.

| Operating System | Sandboxing Technology | Network Policy | Filesystem Policy | Process Isolation |
|---|---|---|---|---|
| **macOS** | `sandbox-exec` (Seatbelt profiles) | Denied by default (`(deny network*)`) | Read-only host, write-access scoped exclusively to ephemeral worktree and tool cache | `setpgid` process groups with hard kill timers |
| **Linux** | Bubblewrap (`bwrap`) | Denied via `--unshare-net` | Mounts isolated root with temporary `/tmp` and workspace overlay | Namespaced PID, IPC, and UTS isolation |
| **Windows / Fallback** | Win32 Job Objects / Process Groups | Firewall-restricted socket hooks | Scoped directory access | Process group termination (`TerminateJobObject`) |

### Offline Pre-Warming Mandate (`artix warm`)
To eliminate the risk of arbitrary dependency downloads during autonomous coding:
1. Sandboxed test execution executes with `--offline` (e.g. `./gradlew test --offline` or `go test -mod=readonly ./...`).
2. Developers pre-warm build caches explicitly using `artix warm`.
3. If an un-cached dependency is requested during execution, the sandbox immediately halts and reports `OFFLINE_CACHE_MISS`, naming the exact missing artifact rather than silently hitting the public Internet.

---

## 4. Default-Deny Active Scan Scope

To prevent automated security fuzzing or testing tools from attacking unauthorized production infrastructure, `kritix-ai` enforces a **fail-closed host allowlist**:

- Environment variable: `KRITIX_ALLOWED_TARGETS`
- Format: Comma-separated domain suffixes or loopback IPs:
  ```bash
  export KRITIX_ALLOWED_TARGETS=".staging.internal.net,localhost,127.0.0.1"
  ```
- **Fail-Closed Rule**: If `KRITIX_ALLOWED_TARGETS` is unset or empty, all network operations (`exec.fuzz`, `exec.owasp-dast`, `perf.latency`, `agent.vision-crawl`) are rejected with an authorization error.
- Any request targeting an IP or hostname outside this list is immediately aborted before TCP handshake initialization.

---

## 5. Tamper-Evident Hash-Chained Audit Ledger (`KRITIX_AUDIT_LOG`)

Every critical authorization check, role transition, model call, and test verdict is appended to a cryptographic audit log:

```json
{
  "sequence": 142,
  "timestamp": "2026-10-10T21:04:12.842Z",
  "actor": "developer_alice",
  "role": "Developer",
  "action": "EXECUTE_BLUEPRINT",
  "target": "pr-smoke-guard",
  "prev_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "record_hash": "7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069",
  "signature": "hmac_sha256_sig_hex..."
}
```

### Verification & Integrity
- Each line contains `prev_hash`, binding it to the preceding entry.
- The `record_hash` is computed as:
  $$\text{Hash}_n = \text{SHA256}(\text{Sequence}_n \parallel \text{Timestamp}_n \parallel \text{Actor}_n \parallel \text{Action}_n \parallel \text{Hash}_{n-1})$$
- The daemon provides a built-in verification tool:
  ```bash
  kritix auth verify-audit
  ```
- If any line is altered, deleted, or reordered, verification fails immediately, and engine daemons refuse to start or append further records.

---

## 6. Pre-Commit Exposure Check (Engine Standard 18)

Dialex Suite enforces mandatory pre-commit scanning across all development repositories:
1. **Internal Local Paths**: Scans for `/Users/...`, `C:\Users\...`, home directory references, or private intranet URLs.
2. **Secrets & Keys**: Scans for RSA/ECDSA private keys, API keys (`sk-...`, `Bearer ...`), tokens, and `.env` credentials.
3. **Personal Data**: Scans for internal email addresses, phone numbers, and employee hardware identifiers.
4. **Enforcement**: Build pipelines fail unconditionally if any staged file violates these checks.

---

*Next: Continue to [04. Prospective Users & Technical Evaluation](../suite-architecture/04_PROSPECTIVE_USERS_TECHNICAL_EVALUATION.md).*
