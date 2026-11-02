# Scribex AI: Master Platform Integration & Setup Guide

**Module:** `scribex-ai`  
**Author:** Keltstone Systems Core Engineering & Knowledge Architecture Group  
**Audience:** Platform Engineers, DevOps Leads, Tech Leads, Documentation Maintainers  
**Scope:** Streamlined 4-pillar integration framework across all documentation sources.

---

## The 4-Pillar Integration Blueprint

Every documentation source supported by Scribex AI follows a unified, 4-step implementation blueprint:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       The 4-Pillar Integration Blueprint                    │
├───────────────────────────────┬─────────────────────────────────────────────┤
│ 1. Initial Implementation     │ `scribex init`, anchors/manifests, local    │
│    in Repository              │ baseline check & dry-run sync.              │
├───────────────────────────────┼─────────────────────────────────────────────┤
│ 2. Doc System Settings        │ Platform tokens, space/project permissions, │
│    & Authentication           │ secrets storage, on-premise private hosts.  │
├───────────────────────────────┼─────────────────────────────────────────────┤
│ 3. CI/CD & PR Gate Settings   │ Pull Request check, zero-token impact gate, │
│                               │ status comments, post-merge living sync.    │
├───────────────────────────────┼─────────────────────────────────────────────┤
│ 4. Scribex Dashboard          │ Metrics tracked in `scribex serve`, team    │
│    & Monitoring Items         │ RBAC mappings, drift alerts, health APIs.   │
└───────────────────────────────┴─────────────────────────────────────────────┘
```

---

## Supported Documentation Sources

Select your documentation source below to view its streamlined, step-by-step implementation guide:

| Documentation Source | Primary Artifacts | Primary Integration Channel | Streamlined Guide |
| :--- | :--- | :--- | :--- |
| **Atlassian Confluence** | Space specs, API guides, RFCs | REST API (`/wiki/rest/api`) | 📘 [Confluence Setup Guide](platforms/confluence.md) |
| **Atlassian Jira** | User stories, acceptance criteria | REST API (`/rest/api/2`) | 📋 [Jira Setup Guide](platforms/jira.md) |
| **In-Repo Markdown & Wikis** | READMEs, ADRs, wikis, Mermaid | In-repo AST & `.wiki.git` | 📝 [Markdown & Wikis Guide](platforms/markdown_and_wikis.md) |
| **GitHub & GitHub Enterprise** | PR reviews, bot comments, GH Wiki | GitHub Actions & API | 🐙 [GitHub Setup Guide](platforms/github.md) |
| **GitLab CI/CD & Self-Hosted** | MR gates, pipelines, wiki | GitLab CI Runner & API | 🦊 [GitLab Setup Guide](platforms/gitlab.md) |
| **Notion Workspaces** | Product PRDs, company wikis | Model Context Protocol (MCP) | 🧠 [Notion Setup Guide](platforms/notion.md) |
| **Compliance & Audit Vaults** | SOC2/ISO 27001 trace matrices | CSV Matrix & JSON sidecar | 🛡️ [Compliance & Audit Guide](platforms/compliance_and_audit.md) |
| **Central Governance Daemon** | Multi-repo health control plane | HTTP Dashboard & REST API | 🖥️ [Governance Dashboard Guide](platforms/governance_dashboard.md) |

---

## Global Quick Reference

### 1. Local Secrets Storage
Scribex stores local secrets in `.scribex/secrets.env` (`0600` permissions, auto-gitignored):
```bash
printf %s "your_secret_token" | scribex secrets set TOKEN_NAME
```

### 2. On-Premise & Private Subnet Routing
When integrating with on-premise instances (Jira Data Center, Confluence Server, GitHub Enterprise Server) on private corporate subnets (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`):
```bash
export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
```
*(Cloud metadata addresses like `169.254.169.254` remain permanently blocked for safety).*
