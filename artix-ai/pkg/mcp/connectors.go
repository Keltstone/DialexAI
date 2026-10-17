package mcp

import (
	"encoding/base64"
	"fmt"
)

// ConnectorConfig configures external MCP tool connectors.
type ConnectorConfig struct {
	Type     string            `json:"type"` // "jira", "postgres", "figma"
	Endpoint string            `json:"endpoint,omitempty"`
	Command  string            `json:"command,omitempty"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Token    string            `json:"token,omitempty"`
}

// JiraConnector creates a client configured for Jira MCP server.
func JiraConnector(host, email, apiToken string) Client {
	endpoint := fmt.Sprintf("https://%s/rest/mcp/v1", host)
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", email, apiToken)))
	headers := map[string]string{
		"Authorization": "Basic " + auth,
	}
	return NewHTTPClient(endpoint, headers)
}

// PostgresConnector creates a stdio client for the Postgres MCP server.
func PostgresConnector(connectionString string) (*StdioClient, error) {
	return NewStdioClient("npx", "-y", "@modelcontextprotocol/server-postgres", connectionString)
}

// FigmaConnector creates an HTTP client for Figma MCP server.
func FigmaConnector(apiKey string) Client {
	headers := map[string]string{
		"X-Figma-Token": apiKey,
	}
	return NewHTTPClient("https://api.figma.com/v1/mcp", headers)
}
