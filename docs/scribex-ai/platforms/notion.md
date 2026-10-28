# Notion Workspace Integration via Model Context Protocol (MCP)

**Module:** `scribex-ai`  
**Platform:** Notion Workspaces & Notion API  
**Audience:** Product Managers, Documentation Architects, AI Engineers, Team Leads  
**Scope:** Notion internal integrations, Model Context Protocol (MCP) configuration, secret isolation, and agentic documentation workflows.

---

## 1. Overview: Bridging Code Anchors with Notion Workspaces

Many fast-growing technology companies and product teams maintain product requirements documents (PRDs), architectural decisions, and onboarding knowledge bases inside **Notion**.

Traditional documentation sync tools struggle with Notion due to its complex block-based data structure, leading to frequent formatting breakage and sync corruption.

Scribex AI solves this by integrating with Notion through the **Model Context Protocol (MCP)**:

```
┌─────────────────────────────────┐           ┌────────────────────────────────┐
│        Scribex Repository       │           │        Notion Workspace        │
│ .scribex/manifest.yaml          │           │ Engineering Wiki & PRDs        │
│ Local AST Anchors & Diagrams    │           │ Databases, Tables, Rich Blocks │
└────────────────┬────────────────┘           └───────────────┬────────────────┘
                 │                                            │
                 ▼                                            ▼
      [ scribex mcp serve ]                      [ @notionhq/notion-mcp-server ]
                 │                                            │
                 └──────────────────────┬─────────────────────┘
                                        ▼
                        [ Agent Client: Claude / Cursor ]
                        Agent queries Scribex AST contracts
                        and writes contextual Notion pages
```

### Why MCP for Notion?
- **Zero In-Repo Bloat:** No heavy Notion SDKs or database pollers running in your repository.
- **Sovereign Secret Control:** Secrets are referenced strictly via `${NOTION_TOKEN}` environment variables in `.scribex/mcp.json`. Literal tokens are rejected before write.
- **Unified Agentic Context:** AI agents (Claude Desktop, Cursor, Antigravity, Socratix Engine) can query code contracts via Scribex and write rich Notion documents in a single deliberation turn.

---

## 2. Setting Up the Notion Internal Integration

To allow MCP to access your Notion workspace:

### Step 1: Create an Internal Integration in Notion
1. Open your browser and navigate to [notion.so/my-integrations](https://www.notion.so/my-integrations).
2. Click **+ New integration**.
3. Fill out the basic settings:
   - **Name:** `Scribex Documentation Bridge`
   - **Associated workspace:** Select your company workspace.
   - **Type:** Internal.
4. Under **Capabilities**, ensure the following are enabled:
   - Read content
   - Update content
   - Insert content
5. Click **Submit** $\rightarrow$ Click **Show** under **Internal Integration Secret** and copy the token (starts with `secret_` or `ntn_`).

---

### Step 2: Grant the Integration Page Access in Notion
By default, Notion integrations have zero access to workspace pages until explicitly shared:
1. Open the top-level Engineering or Product page in your Notion workspace.
2. Click the **...** menu in the top-right corner.
3. Scroll down and click **Connections** (or **Add connections**).
4. Select `Scribex Documentation Bridge`.
5. All sub-pages and databases under this parent page now inherit access.

---

## 3. Configuring Notion in Scribex

### Step 1: Store the Notion Token in Scribex Secrets
Save the secret to your local encrypted `.scribex/secrets.env` (0600 permissions, auto-gitignored):

```bash
printf %s "secret_your_notion_token_here" | scribex secrets set NOTION_TOKEN
```

Verify that the secret is stored:
```bash
scribex secrets list
# Output: NOTION_TOKEN (set, value hidden)
```

---

### Step 2: Add Notion to `.scribex/mcp.json` Using the Built-In Preset

Run:
```bash
scribex mcp add notion -preset notion
```

This registers the vetted upstream Notion MCP server in `.scribex/mcp.json`:

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

> **Security Rule:** Notice that the configuration references `${NOTION_TOKEN}` instead of storing the literal token. Scribex strictly validates configuration files and errors out if literal secrets are detected.

---

### Step 3: Validate the MCP Configuration

Run:
```bash
scribex mcp check
```

Output:
```
notion             npx  [ok]
scribex            scribex  [ok]
```

If the environment variable is missing, `scribex mcp check` reports `[unset: NOTION_TOKEN]` and exits with code `1`.

---

## 4. Connecting Agent Clients (Claude Desktop, Cursor, Antigravity)

Export your configured MCP servers to your agent client:

```bash
# Print the drop-in JSON configuration:
scribex mcp export
```

### 4.1 Claude Desktop Configuration
Add the exported output to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "scribex": {
      "command": "scribex",
      "args": ["mcp", "serve", "-C", "/path/to/your/repo"]
    },
    "notion": {
      "command": "npx",
      "args": ["-y", "@notionhq/notion-mcp-server"],
      "env": {
        "NOTION_TOKEN": "secret_your_actual_token_here"
      }
    }
  }
}
```

### 4.2 Cursor IDE Integration
In Cursor:
1. Open **Cursor Settings** $\rightarrow$ **Features** $\rightarrow$ **MCP**.
2. Click **+ Add New MCP Server**.
3. Name: `Scribex`. Type: `command`. Command: `scribex mcp serve -C .`.
4. Name: `Notion`. Type: `command`. Command: `npx -y @notionhq/notion-mcp-server`.

---

## 5. Daily Agentic Workflows

With both Scribex and Notion active in your agent client, you can execute powerful multi-tier workflows in natural language:

### Example Prompt to AI Agent:
> *"Check Scribex for any stale anchors in our authentication module. For any public method whose signature changed, read the corresponding Notion PRD page titled 'Auth v2 Requirements' and draft a revised technical summary reflecting the code's active signature."*

**Execution Steps Taken by the Agent:**
1. Calls `scribex_check` MCP tool $\rightarrow$ Retrieves stale anchors in `pkg/auth/jwt.go`.
2. Calls Notion MCP tool `search` $\rightarrow$ Locates the 'Auth v2 Requirements' page.
3. Compares the active code signature against the Notion specification.
4. Generates a coherent, synchronized update.
