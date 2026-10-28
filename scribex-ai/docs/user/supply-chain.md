# Scribex AI Supply Chain Security & Evidence

This document details the supply-chain security controls, automated evidence generation, and release verification procedures for Scribex AI.

## Evidence Guarantees: What Each Check Proves and Does Not Prove

| Check / Artifact | Tool | What It Proves | What It Does NOT Prove |
|---|---|---|---|
| **Vulnerability Scanning** | `govulncheck ./...` | Proves that no compiled Go code or reachable call paths contain known vulnerabilities documented in the Go Vulnerability Database (`vuln.go.dev`). | Does not prove absence of zero-day vulnerabilities, logic flaws, or vulnerabilities in uncalled dependency functions. |
| **Dependency Auditing** | `osv-scanner --lockfile=go.mod` | Proves that declared `go.mod` package versions do not match known Open Source Vulnerabilities (OSV) records. | Does not prove safety of private code, vendored packages, or undiscovered security flaws. |
| **Software Bill of Materials (SBOM)** | `cyclonedx-gomod app -main cmd/scribex` (`sbom.cdx.json`) | Proves complete, standardized inventory of application components, direct/indirect dependencies, and the exact Go toolchain version used to compile the binary. | An SBOM is an inventory, not a safety certification. It does not prove that components are bug-free. |
| **Cryptographic Signatures** | `cosign sign-blob` (Keyless GitHub OIDC) | Proves cryptographic authenticity and provenance: the binary was built directly by GitHub Actions on the official tag in this repository without tampering. | Does not prove code safety: signatures certify provenance and origin, not safety or correctness. |
| **Build Provenance Attestation** | `actions/attest-build-provenance` (SLSA Level 3) | Proves builder authenticity via cryptographically signed build provenance attestation stored in GitHub Attestation registry. | Does not attest to runtime host environment safety outside the build pipeline. |
| **Build Reproducibility** | `scripts/build.sh` across 3 runner instances | Proves that compiling with `CGO_ENABLED=0 -trimpath -buildvcs=false` and fixed `SOURCE_DATE_EPOCH` produces byte-identical SHA-256 hashes across independent Ubuntu runners and macOS cross-compilation. | Does not prove compiler toolchain trustworthiness (reflections of trusting trust). |
| **Zero-Token Performance Bounds** | `scripts/perf-gen.sh` synthetic benchmark (`perf.md`) | Proves that unchanged symbol checks on 1k, 10k, and 100k file synthetic codebases bypass AI evaluation with zero tokens and take under 2s median on 10k files. | Does not guarantee performance on non-standard network mounts or pathological git topologies. |

## How Customers and Auditors Verify a Release

Bank security auditors and consumers can verify any Scribex release binary without trusting local build environments.

### Automated Verification via `verify-release.sh`

Run the verification script against the target release tag:

```bash
./scripts/verify-release.sh scribex-v1.0.0
```

The script performs the following end-to-end verifications:
1. Downloads release assets (`dist/scribex_*`, `SHA256SUMS`, signatures, and `.bundle` files) using GitHub CLI (`gh release download`).
2. Validates all binary checksums against `SHA256SUMS` using `sha256sum -c`.
3. Verifies Sigstore/Cosign keyless signatures against the GitHub Actions OIDC identity:
   - Expected Issuer: `https://token.actions.githubusercontent.com`
   - Expected Subject: `https://github.com/Keltstone/DialexAI/.github/workflows/scribex.yml@refs/tags/<TAG>`
4. Prints `PASS` on successful verification or `FAIL` with details on any discrepancy.

### Manual Verification

If running verification manually:

1. **Verify Checksums**:
   ```bash
   sha256sum -c SHA256SUMS
   ```

2. **Verify Keyless Cosign Signature**:
   ```bash
   cosign verify-blob \
     --bundle SHA256SUMS.bundle \
     --certificate-identity-regexp "^https://github\.com/Keltstone/DialexAI/\.github/workflows/scribex\.yml@refs/tags/scribex-v" \
     --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
     SHA256SUMS
   ```

3. **Verify GitHub SLSA Build Provenance**:
   ```bash
   gh attestation verify scribex_linux_amd64 --repo Keltstone/DialexAI
   ```

## Where Artifacts Live

All supply-chain security artifacts are published directly with each release on GitHub:
- **Binaries**: Multi-platform executables (`scribex_linux_amd64`, `scribex_linux_arm64`, `scribex_darwin_amd64`, `scribex_darwin_arm64`).
- **Checksums**: `SHA256SUMS`.
- **Signatures & Bundles**: `${asset}.bundle`, `${asset}.sig`, `${asset}.cert`.
- **SBOM**: `sbom.cdx.json` (CycloneDX JSON format).
- **Vulnerability Report**: `govulncheck.txt`.
- **Benchmark Report**: `perf.md`.
- **Attestation Registry**: Hosted in GitHub's tamper-evident attestation store.
