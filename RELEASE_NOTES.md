# Artix Enterprise Release Notes — v0.11.0-enterprise

## Release Summary
Artix Enterprise `v0.11.0-enterprise` delivers comprehensive hostile reviewer remediations across security boundaries, test retention gates, AST integrity analysis, sandbox child environment isolation, taint tracking expansions, cryptographic trust verification, and benign corpus evaluations (R14-1 – R14-9):

- **R14-1: Test Retention & Regression Gate & CLI Restoration**:
  - Restored CLI unit tests for `verify-approval` and the deprecated `merge` alias.
  - Added CI check (`scripts/check-test-retention.sh`) that compares the test function inventory against the previous release tag baseline, failing closed if any `Test*` function is removed without documentation in `REMOVED_TESTS.md`.
  - Added explicit test cases rejecting unreachable dead switch cases.

- **R14-2: AST Adversarial Reviewer Hardening**:
  - Enforced `errgroup.Go` wait requirement: flags uncoordinated `errgroup.Go` calls lacking an enclosing or subsequent `Wait()`.
  - Treated unconditional or constant-true `t.Skip*`/`t.SkipNow` before the first assertion as no reachable assertion.
  - Flagged testify `assert.Len`, `assert.Empty`, `assert.Contains` calls on literal empties as tautological assertions.
  - Treated nil-channel select cases as dead code paths.
  - Preserved float `x != x` comparisons without flagging as tautology to correctly support IEEE-754 `NaN` checks.

- **R14-3: Taint Analysis & Egress Expansion**:
  - Expanded taint sink detection to standard library networking packages: `net/smtp`, `net/rpc`, and `net/textproto`.
  - Added secret exfiltration detection for `database/sql` external host DSNs.
  - Tracked method-chain flows, struct receiver field mutations, and map store/load operations across AST paths.

- **R14-4: Sandbox Environment Isolation & Canary Leak Prevention**:
  - Built child process execution environment strictly from an allowlist (`PATH`, `HOME=<sandbox-temp>`, `LANG`, `GOPATH`, `GOROOT`, `GOCACHE`, `TMPDIR`, etc.).
  - Stripped all provider, forge, audit, enterprise, and sensitive credentials from child process environments.
  - Implemented post-test workspace scan detecting canary secret leaks in non-allowlisted files prior to commit.

- **R14-5: Trust Anchor & Build Reproducibility**:
  - Published signing key fingerprint (`SHA256:Dh5vIjePKsm29xfvnLukrKDTOzLM/w5u6rYBcB3hmNg`) at resolving `.well-known/security.txt`.
  - Signed unified `SHA256SUMS` with SSH signature (`ssh-keygen -Y sign`) including binaries, `.vsix`, and `.zip` distribution packages.
  - Standardized reproducible compilation flags (`-trimpath`, `-ldflags="-s -w"`) on pinned Go toolchains.

- **R14-6: Fail-Closed Tag Registry Remote Verification**:
  - Replaced ad-hoc verification with `scripts/verify-tag-registry.sh` validating against committed `docs/release/TAG_IMMUTABILITY_REGISTRY.json` and querying remote tag refs via `git ls-remote --tags origin`.
  - Fails closed immediately if remote refs are offline or empty.
  - Clarified immutability assurances in release documentation.

- **R14-7: Release Hygiene & Verify-Only Terminology**:
  - Standardized `verify-approval` across CLI commands, help text, and documentation.
  - Retained `artix merge` strictly as a deprecated alias directing users to `verify-approval`.

- **R14-8: Full Enterprise Phase 1 to Verify-Approval Matrix**:
  - Implemented full enterprise end-to-end integration test (`TestEnterprisePhase1ToVerifyApprovalE2E`) with compiled trust keys and root policies without requiring `ARTIX_ENTERPRISE`.
  - Tested negative matrix cases: stale head SHA, self-approval refusal, bot approval rejection, dismissed reviews, and forge 5xx errors.

- **R14-9: Three-Source Benign Corpus Evaluation with Absolute FPR Targets**:
  - Evaluated against standard library GOROOT (1,808 files) and pinned ecosystem modules (Set 1: `golang.org/x/sync`, `github.com/google/uuid`, `go.uber.org/zap`, `golang.org/x/crypto`, `github.com/stretchr/testify`; Set 2: `github.com/spf13/cobra`, `golang.org/x/sys`, `github.com/pkg/errors`).
  - Achieved: GOROOT FPR 4.81% (Target <= 5.0%), Pinned Set 1 combined FPR 1.73% (Target <= 5.0%), testify FPR 8.33% (Target <= 10.0%).
  - Added absolute target gate and gap reporting in `cmd/artix-corpus/main.go`.

---

## Cryptographic Trust Root & Signatures

### Release Signing Key
- **Signer Identity / Principal**: `releases@artix.ai`
- **Tagger Identity**: `releases@artix.ai <releases@artix.ai>`
- **Key Type**: `ED25519 (SSH format)`
- **Public Key**: `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ4FO6aRcfHA07XqA9SGUUfpiqYfLsaRz+KUwigN2K5x`
- **Key Fingerprint (SHA-256)**: `SHA256:Dh5vIjePKsm29xfvnLukrKDTOzLM/w5u6rYBcB3hmNg`

### Verifying the Tag
To independently verify the cryptographic signature on this tag:
```bash
TEMP_SIGNERS=$(mktemp)
echo "releases@artix.ai ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ4FO6aRcfHA07XqA9SGUUfpiqYfLsaRz+KUwigN2K5x" > "$TEMP_SIGNERS"
git -c gpg.ssh.allowedSignersFile="$TEMP_SIGNERS" tag -v v0.11.0-enterprise
rm -f "$TEMP_SIGNERS"
```

### Tag Immutability Registry
Exact commit targets verified via `git rev-parse <tag>^{commit}` and `git rev-parse <tag>`:
- **`v0.1.0-enterprise`**: `92fc42fa5a2f694c3d2f8f4a556d35a529bdc09b` (tag: `d42da85a32b4905e62d4b980bb6d3320523efd7d`)
- **`v0.2.0-enterprise`**: `7b4164992fd24a9e6154db382b2d8d87f68cab89` (tag: `51c215a385ded1e206fcf01d461549e97bcf76f6`)
- **`v0.3.0-enterprise`**: `22a07738bbbcbd4b93398801292eb9f9c98299ee` (tag: `9384732de2a0ffae6aae71935e857ad7c842bed1`)
- **`v0.4.0-enterprise`**: `3cb016d11557268d8d0050d461d62f342fc102c2` (tag: `aaaf821939ab414d593b763d806a509b7bdced85`)
- **`v0.5.0-enterprise`**: `ea197f8108a8824c64364a2c059a3e261475073f` (tag: `71fbb86f8c0547e749811da19a74fd80bcc6a0cb`)
- **`v0.6.0-enterprise`**: `e1803c1553c440353051385f304109b9551c992f` (tag: `30d4c0559ea1eacdeb051ab399de46fe10cf4604`)
- **`v0.7.0-enterprise`**: `5d31de9c95c4980ed8ff019fb966449acfd085c6` (tag: `091bed22e42db575235a7b35c99dcc42b380b534`)
- **`v0.8.0-enterprise`**: `c1aa48e44172283eb0977fc11841a0ddce2edab8` (tag: `a2ccd59289ed05104554a458c268171d3fe58919`)
- **`v0.9.0-enterprise`**: `8a39e5ce06e36abc08f7380cd5e0ad2e11a09ae0` (tag: `2d4ba1c4be4c32c8f6e0d8105cfbc4f73ba08d49`)
- **`v0.10.0-enterprise`**: `4b7892495515bc779a9434be591da363b467315f` (tag: `4e4cae598feaaa60f8430d6dbf03595296d0064f`)

---

## Clean Clone Verification Commands
On a clean clone of the repository:
```bash
# 1. Verify build
go build ./...

# 2. Verify vet
go vet ./...

# 3. Verify all tests with race detector
go test -race -count=1 ./...

# 4. Verify test retention gate against baseline tag
./scripts/check-test-retention.sh

# 5. Run reproducible 3-source benign corpus evaluation (GOROOT + pinned modules + fixed modules)
./scripts/benign-corpus.sh

# 6. Verify committed tag registry against remote refs (fails closed if remote is unreachable)
./scripts/verify-tag-registry.sh

# 7. Verify tag signature independently against published fingerprint in .well-known/security.txt
TEMP_SIGNERS=$(mktemp)
echo "releases@artix.ai ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ4FO6aRcfHA07XqA9SGUUfpiqYfLsaRz+KUwigN2K5x" > "$TEMP_SIGNERS"
ssh-keygen -lf "$TEMP_SIGNERS" | grep -F "SHA256:Dh5vIjePKsm29xfvnLukrKDTOzLM/w5u6rYBcB3hmNg" || { echo "Fingerprint mismatch!"; exit 1; }
git -c gpg.ssh.allowedSignersFile="$TEMP_SIGNERS" tag -v v0.11.0-enterprise
rm -f "$TEMP_SIGNERS"
```
