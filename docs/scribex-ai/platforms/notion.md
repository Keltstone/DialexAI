# Notion Workspace Setup Guide (via Model Context Protocol)

**Module:** `scribex-ai`  
**Platform:** Notion Workspaces & Notion API  
**Audience:** Product Managers, Documentation Architects, AI Engineers  
**Scope:** Streamlined setup: repo implementation, Notion settings, agent PR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Add Notion MCP Server
Register the vetted Notion Model Context Protocol server in `.scribex/mcp.json`:
```bash
scribex mcp add notion -preset notion
```

This populates `.scribex/mcp.json` with the upstream server specification:
```json
{
  "mcpServers": {
    "notion": {
      "command": "npx",
      "args": ["-y", "@notionhq/notion-mcp-server"],
      "env": {
        "NOTION_TOKEN": "${NOTION_TOKEN}"
      }
    }
  }
}
```
*Notice that credentials are only referenced via `${NOTION_TOKEN}` to prevent accidental secret commits.*

### 1.2 Verify MCP Server Readiness
Check if all required environment variables are set:
```bash
scribex mcp check
```
If `NOTION_TOKEN` is unset, Scribex alerts you to set it before use.

---

## Step 2: Notion Settings & Authentication

### 2.1 Create an Internal Integration in Notion
1. Navigate to [notion.so/my-integrations](https://www.notion.so/my-integrations).
2. Click **+ New integration**.
3. Name: `Scribex Documentation Bridge`.
4. Under **Capabilities**, ensure **Read content**, **Update content**, and **Insert content** are enabled.
5. Click **Submit** $\rightarrow$ copy the **Internal Integration Secret** (`secret_...`).

### 2.2 Share Workspace Pages with the Integration
In Notion:
1. Open the top-level Engineering or Product documentation page.
2. Click **...** (top right) $\rightarrow$ **Connections** (or **Add connections**).
3. Select `Scribex Documentation Bridge`. All sub-pages inherit access automatically.

### 2.3 Store Notion Token in Scribex Secrets
Store the secret in Scribex's local vault (`0600` permissions, auto-gitignored):
```bash
printf %s "secret_your_notion_token" | scribex secrets set NOTION_TOKEN
```

Verify that the secret is stored:
```bash
scribex secrets list
# Output: NOTION_TOKEN (set, value hidden)
```

---

## Step 3: CI/CD & Agent PR Gate Settings

Unlike static wikis, Notion integrations often operate through **Agentic Workflows** (Claude Desktop, Cursor, Antigravity) running automated verification gates.

### 3.1 Export MCP Configuration for Agent Clients
Export the configuration to your agent client:
```bash
# Print drop-in JSON configuration:
scribex mcp export
```

Add the exported configuration to:
- **Claude Desktop:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Cursor IDE:** **Settings** $\rightarrow$ **Features** $\rightarrow$ **MCP** $\rightarrow$ **Add New MCP Server**.

### 3.2 Agent PR Gate Workflow
In agent-assisted CI or local pre-commit reviews, prompt your agent:
> *"Run `scribex_check` to detect stale anchors. If any public API changed, query Notion for the corresponding PRD and draft a synchronized update for review."*

---

## Step 4: Scribex Dashboard Items

When monitored via `scribex serve`, Notion MCP integration appears under the governance control plane:

### 4.1 Dashboard Overview Items
- **MCP Servers Health:** Validates that configured MCP servers in `.scribex/mcp.json` have valid commands and environment bindings.
- **Secret Presence Check:** In the `/config` view, displays whether `NOTION_TOKEN` is `set` or `unset` (values remain strictly masked).
- **Tool Expiry Status:** Confirms that local Scribex MCP tools (`scribex_check`, `scribex_wiki_lint`) are available to connected agents.
