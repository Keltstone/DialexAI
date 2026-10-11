# Independent Pilot Evaluation Rubric & Audit Protocol

**Document Version:** 2.2.0-enterprise  
**Classification:** Hostile Reviewer Acceptance Artifact  
**Governance Scope:** Evaluator-Owned Verification of Autonomous & Supervised Task Executions  

---

## 1. Evaluation Methodology & Verification Protocol

The pilot evaluation records the end-to-end execution of real engineering tasks across multiple distinct open-source codebases not owned, authored, or controlled by the vendor (*artix-ai*, *kritix-ai*, or *socratix-engine*).

Every task record must be fully joinable and verifiable across four independent cryptographic and provenance dimensions:

1. **Story Specification ID (`specId`)**: Traceable to the stakeholder council story spec.
2. **Git Commit SHA (`commitSHA`)**: Resolvable via `git cat-file -e <sha>^{commit}` to a real commit object in a clone of the target repository.
3. **Audit Event Hash (`auditRecordHash`)**: A 64-character hex SHA-256 hash identifying the immutable, hash-chained convergence audit record signed by the enterprise audit logger.
4. **Ledger Reference (`ledgerRef`)**: The file-locked token/financial ledger entry recording provider-reported usage and USD reconciliation.

Self-graded records, records targeting vendor-controlled repositories, and records evaluated by unidentifiable aliases are strictly rejected and discarded by the pilot harness ([`pilot.go`](file:///Users/arunelectra/Project/Tech/KMP/Kolta/DialexAI/artix-ai/pkg/pilot/pilot.go)).

---

## 2. Evaluation Rubric & Rejection Criteria

Each task outcome is classified according to the following strict criteria:

1. **Accepted (`accepted: true`)**:
   - All acceptance criteria scenarios in the spec pass in the isolated sandbox.
   - Adversarial code reviewer signs off with zero blocking security taboos, no test deletions, no gutted assertions, and non-decreasing test count.
   - Code changes cleanly merge into the target branch without syntax or build failures.

2. **Rejected (`accepted: false`, `falseRejection: false`)**:
   - The generated patch introduced a real defect, violated AST taboos, broke existing unit tests, or exceeded token/cost caps.

3. **False Rejection (`accepted: false`, `falseRejection: true`)**:
   - The patch was semantically correct and passed all sandbox tests, but was blocked by an overly conservative pre-filter rule or transient external API issue.

4. **Human Edit Distance (`humanEditDistance`)**:
   - Levenshtein character edit distance between the patch output and the final human-reviewed commit. An edit distance of 0 denotes complete zero-touch autonomous acceptance.

