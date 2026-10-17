package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHTTPClient_FullLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			res := InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo: ServerInfo{
					Name:    "mock-mcp-server",
					Version: "1.0",
				},
			}
			resBytes, _ := json.Marshal(res)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  resBytes,
			})
		case "tools/list":
			tools := []MCPTool{
				{
					Name:        "query_database",
					Description: "Executes a read-only SQL query",
				},
			}
			resBytes, _ := json.Marshal(map[string]interface{}{"tools": tools})
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  resBytes,
			})
		case "tools/call":
			callRes := ToolCallResult{
				Content: []ToolContent{
					{
						Type: "text",
						Text: `{"rows": [{"id": 1, "name": "Alice"}]}`,
					},
				},
			}
			resBytes, _ := json.Marshal(callRes)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  resBytes,
			})
		case "resources/list":
			resources := []MCPResource{
				{
					URI:  "jira://PROJ-123",
					Name: "PROJ-123: Fix payment bug",
				},
			}
			resBytes, _ := json.Marshal(map[string]interface{}{"resources": resources})
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  resBytes,
			})
		case "resources/read":
			contents := []ResourceContent{
				{
					URI:  "jira://PROJ-123",
					Text: "Issue details: payment gateway timeout handling",
				},
			}
			resBytes, _ := json.Marshal(map[string]interface{}{"contents": contents})
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  resBytes,
			})
		default:
			http.Error(w, "unknown method", 404)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, map[string]string{
		"X-Test-Auth": "token-123",
	})
	ctx := context.Background()

	// 1. Initialize
	initRes, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	if initRes.ServerInfo.Name != "mock-mcp-server" {
		t.Errorf("unexpected server name: %s", initRes.ServerInfo.Name)
	}

	// 2. Tools
	tools, err := client.ListTools(ctx)
	if err != nil || len(tools) != 1 {
		t.Fatalf("unexpected tools: %v (err: %v)", tools, err)
	}
	if tools[0].Name != "query_database" {
		t.Errorf("unexpected tool name: %s", tools[0].Name)
	}

	callRes, err := client.CallTool(ctx, "query_database", map[string]interface{}{"sql": "SELECT 1"})
	if err != nil || len(callRes.Content) == 0 {
		t.Fatalf("failed tool call: %v (err: %v)", callRes, err)
	}

	// 3. Resources
	resources, err := client.ListResources(ctx)
	if err != nil || len(resources) != 1 {
		t.Fatalf("unexpected resources: %v (err: %v)", resources, err)
	}

	resContent, err := client.ReadResource(ctx, "jira://PROJ-123")
	if err != nil || resContent.Text == "" {
		t.Fatalf("failed read resource: %v (err: %v)", resContent, err)
	}
}

func TestConnectors_Creation(t *testing.T) {
	jiraClient := JiraConnector("acme.atlassian.net", "bot@acme.com", "token")
	if jiraClient == nil {
		t.Errorf("expected jira client")
	}

	figmaClient := FigmaConnector("figma-pat")
	if figmaClient == nil {
		t.Errorf("expected figma client")
	}

	cfg := ConnectorConfig{
		Type:     "postgres",
		Endpoint: "postgres://localhost:5432/db",
		Command:  "npx",
		Args:     []string{"-y", "@modelcontextprotocol/server-postgres"},
		Env:      map[string]string{"PGUSER": "postgres"},
		Token:    "secret",
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	var decoded ConnectorConfig
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if decoded.Type != "postgres" || decoded.Token != "secret" {
		t.Errorf("mismatched decoded config: %+v", decoded)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		var result json.RawMessage
		var rpcErr *JSONRPCError

		switch req.Method {
		case "initialize":
			res := InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo:      ServerInfo{Name: "stdio-mock", Version: "1.0"},
			}
			result, _ = json.Marshal(res)
		case "tools/list":
			tools := []MCPTool{{Name: "stdio_tool", Description: "stdio tool desc"}}
			result, _ = json.Marshal(map[string]interface{}{"tools": tools})
		case "tools/call":
			var toolParams ToolCallParams
			_ = json.Unmarshal(req.Params, &toolParams)
			if toolParams.Name == "error_tool" {
				rpcErr = &JSONRPCError{Code: -32603, Message: "stdio internal error"}
			} else {
				callRes := ToolCallResult{Content: []ToolContent{{Type: "text", Text: "stdio-call-ok"}}}
				result, _ = json.Marshal(callRes)
			}
		case "resources/list":
			resources := []MCPResource{{URI: "stdio://res1", Name: "Resource 1"}}
			result, _ = json.Marshal(map[string]interface{}{"resources": resources})
		case "resources/read":
			var resParams struct {
				URI string `json:"uri"`
			}
			_ = json.Unmarshal(req.Params, &resParams)
			if resParams.URI == "empty_resources" {
				contents := []ResourceContent{}
				result, _ = json.Marshal(map[string]interface{}{"contents": contents})
			} else {
				contents := []ResourceContent{{URI: "stdio://res1", Text: "resource payload"}}
				result, _ = json.Marshal(map[string]interface{}{"contents": contents})
			}
		default:
			rpcErr = &JSONRPCError{Code: -32601, Message: "method not found"}
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
			Error:   rpcErr,
		}
		respBytes, _ := json.Marshal(resp)
		os.Stdout.Write(append(respBytes, '\n'))
	}
	os.Exit(0)
}

func TestStdioClient_FullLifecycle(t *testing.T) {
	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	client, err := NewStdioClient(os.Args[0], "-test.run=TestHelperProcess", "--")
	if err != nil {
		t.Fatalf("failed to start stdio client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Initialize
	initRes, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize stdio client: %v", err)
	}
	if initRes.ServerInfo.Name != "stdio-mock" {
		t.Errorf("unexpected server name: %s", initRes.ServerInfo.Name)
	}

	// 2. List tools
	tools, err := client.ListTools(ctx)
	if err != nil || len(tools) != 1 {
		t.Fatalf("unexpected tools: %v (err: %v)", tools, err)
	}
	if tools[0].Name != "stdio_tool" {
		t.Errorf("unexpected tool name: %s", tools[0].Name)
	}

	// 3. Call tool
	callRes, err := client.CallTool(ctx, "stdio_tool", map[string]interface{}{"arg": "val"})
	if err != nil || len(callRes.Content) == 0 {
		t.Fatalf("failed call tool: %v (err: %v)", callRes, err)
	}
	if callRes.Content[0].Text != "stdio-call-ok" {
		t.Errorf("unexpected tool content: %s", callRes.Content[0].Text)
	}

	// 4. List resources
	resources, err := client.ListResources(ctx)
	if err != nil || len(resources) != 1 {
		t.Fatalf("unexpected resources: %v (err: %v)", resources, err)
	}

	// 5. Read resource
	resContent, err := client.ReadResource(ctx, "stdio://res1")
	if err != nil || resContent.Text != "resource payload" {
		t.Fatalf("unexpected resource content: %v (err: %v)", resContent, err)
	}
}

func TestStdioClient_ErrorsAndTimeout(t *testing.T) {
	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	client, err := NewStdioClient(os.Args[0], "-test.run=TestHelperProcess", "--")
	if err != nil {
		t.Fatalf("failed to start stdio client: %v", err)
	}
	defer client.Close()

	// RPC error
	ctx := context.Background()
	_, err = client.CallTool(ctx, "error_tool", nil)
	if err == nil {
		t.Errorf("expected rpc error for error_tool, got nil")
	}

	// Empty resource read error
	_, err = client.ReadResource(ctx, "empty_resources")
	if err == nil {
		t.Errorf("expected error reading empty resource, got nil")
	}

	// Context cancellation / timeout
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = client.CallTool(cancelledCtx, "stdio_tool", nil)
	if err == nil {
		t.Errorf("expected context cancellation error, got nil")
	}
}

func TestStdioClient_InvalidCommand(t *testing.T) {
	_, err := NewStdioClient("non_existent_binary_xyz_12345")
	if err == nil {
		t.Errorf("expected error starting non-existent command, got nil")
	}
}

func TestHTTPClient_ErrorResponses(t *testing.T) {
	// 1. HTTP 500 error
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server500.Close()

	client500 := NewHTTPClient(server500.URL, nil)
	_, err := client500.Initialize(context.Background())
	if err == nil {
		t.Errorf("expected error from 500 status code, got nil")
	}

	// 2. MCP JSON-RPC error
	serverRPCError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error: &JSONRPCError{
				Code:    -32000,
				Message: "forbidden access",
			},
		})
	}))
	defer serverRPCError.Close()

	clientRPCError := NewHTTPClient(serverRPCError.URL, nil)
	_, err = clientRPCError.ListTools(context.Background())
	if err == nil {
		t.Errorf("expected error from RPC error response, got nil")
	}

	// 3. Empty resources error
	serverEmptyRes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resBytes, _ := json.Marshal(map[string]interface{}{"contents": []ResourceContent{}})
		_ = json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  resBytes,
		})
	}))
	defer serverEmptyRes.Close()

	clientEmptyRes := NewHTTPClient(serverEmptyRes.URL, nil)
	_, err = clientEmptyRes.ReadResource(context.Background(), "test://none")
	if err == nil {
		t.Errorf("expected error for empty resource contents, got nil")
	}

	// 4. Close client
	if err := clientEmptyRes.Close(); err != nil {
		t.Errorf("unexpected error on Close: %v", err)
	}
}

func TestP2_JiraConnector_Base64Auth(t *testing.T) {
	client := JiraConnector("example.atlassian.net", "user@example.com", "my-secret-token")
	httpCli, ok := client.(*HTTPClient)
	if !ok {
		t.Fatalf("expected *HTTPClient, got %T", client)
	}
	expectedAuth := "Basic dXNlckBleGFtcGxlLmNvbXpteS1zZWNyZXQtdG9rZW4="
	actualAuth := httpCli.headers["Authorization"]
	if actualAuth != expectedAuth {
		t.Fatalf("Jira Authorization header not base64 encoded! expected %q, got %q", expectedAuth, actualAuth)
	}
}

func TestP2_StdioClient_SanitizesChildEnv(t *testing.T) {
	t.Setenv("ARTIX_FORGE_WEBHOOK_SECRET", "super-secret-forge-token")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "sensitive-aws-key")

	client, err := NewStdioClient("echo", "test")
	if err != nil {
		t.Fatalf("failed to create stdio client: %v", err)
	}
	defer client.Close()

	if client.cmd.Env == nil {
		t.Fatalf("SECURITY FLAW: client.cmd.Env is nil, inherits full unsanitized parent environment!")
	}
	for _, envVar := range client.cmd.Env {
		if strings.HasPrefix(envVar, "ARTIX_FORGE_WEBHOOK_SECRET=") || strings.HasPrefix(envVar, "AWS_SECRET_ACCESS_KEY=") {
			t.Fatalf("SECURITY LEAK: sensitive environment variable leaked into MCP stdio child env: %s", envVar)
		}
	}
}

