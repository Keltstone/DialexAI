package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"artix/pkg/sandbox"
)

// Client is the interface to an MCP server.
type Client interface {
	Initialize(ctx context.Context) (*InitializeResult, error)
	ListTools(ctx context.Context) ([]MCPTool, error)
	CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error)
	ListResources(ctx context.Context) ([]MCPResource, error)
	ReadResource(ctx context.Context, uri string) (*ResourceContent, error)
	Close() error
}

// Stdio MCP Client
// StdioClient launches a local process and communicates via JSON-RPC 2.0 lines over stdio.
type StdioClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	reqID     uint64
	pending   map[uint64]chan *JSONRPCResponse
	pendingMu sync.Mutex
	closeOnce sync.Once
}

// NewStdioClient starts a sub-process (e.g. `npx -y @modelcontextprotocol/server-postgres ...`) as an MCP server.
func NewStdioClient(command string, args ...string) (*StdioClient, error) {
	cmd := exec.Command(command, args...)
	extraEnv := make(map[string]string)
	if v := os.Getenv("GO_WANT_HELPER_PROCESS"); v != "" {
		extraEnv["GO_WANT_HELPER_PROCESS"] = v
	}
	cmd.Env = sandbox.BuildSanitizedChildEnv("", extraEnv)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to start mcp process: %w", err)
	}

	sc := &StdioClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewScanner(stdoutPipe),
		pending: make(map[uint64]chan *JSONRPCResponse),
	}

	go sc.listen()
	return sc, nil
}

func (c *StdioClient) listen() {
	for c.stdout.Scan() {
		line := c.stdout.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		var id uint64
		switch v := resp.ID.(type) {
		case float64:
			id = uint64(v)
		case int:
			id = uint64(v)
		}

		c.pendingMu.Lock()
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()

		if ok && ch != nil {
			ch <- &resp
		}
	}
}

func (c *StdioClient) sendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	id := atomic.AddUint64(&c.reqID, 1)

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		rawParams = b
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json-rpc request: %w", err)
	}

	respCh := make(chan *JSONRPCResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	reqBytes = append(reqBytes, '\n')
	if _, err := c.stdin.Write(reqBytes); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("failed to write to server stdin: %w", err)
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	}
}

func (c *StdioClient) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: ClientInfo{
			Name:    "artix",
			Version: "1.0.0",
		},
		Capabilities: map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
	}

	resp, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var res InitializeResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse initialize response: %w", err)
	}
	return &res, nil
}

func (c *StdioClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse tools/list response: %w", err)
	}
	return res.Tools, nil
}

func (c *StdioClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	params := ToolCallParams{
		Name:      name,
		Arguments: args,
	}

	resp, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var res ToolCallResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse tools/call response: %w", err)
	}
	return &res, nil
}

func (c *StdioClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	resp, err := c.sendRequest(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Resources []MCPResource `json:"resources"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse resources/list response: %w", err)
	}
	return res.Resources, nil
}

func (c *StdioClient) ReadResource(ctx context.Context, uri string) (*ResourceContent, error) {
	params := map[string]string{
		"uri": uri,
	}

	resp, err := c.sendRequest(ctx, "resources/read", params)
	if err != nil {
		return nil, err
	}

	var res struct {
		Contents []ResourceContent `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse resources/read response: %w", err)
	}
	if len(res.Contents) == 0 {
		return nil, fmt.Errorf("no content found for resource: %s", uri)
	}
	return &res.Contents[0], nil
}

func (c *StdioClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			err = c.cmd.Process.Kill()
		}
	})
	return err
}

// HTTP JSON-RPC MCP Client
// HTTPClient connects to remote or HTTP-based MCP endpoints.
type HTTPClient struct {
	endpoint   string
	httpClient *http.Client
	reqID      uint64
	headers    map[string]string
}

// NewHTTPClient creates an HTTP MCP client.
func NewHTTPClient(endpoint string, headers map[string]string) *HTTPClient {
	return &HTTPClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		headers: headers,
	}
}

func (c *HTTPClient) sendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	id := atomic.AddUint64(&c.reqID, 1)

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		rawParams = b
	}

	reqPayload := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http error: status %d", resp.StatusCode)
	}

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode json-rpc response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp error (%d): %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, nil
}

func (c *HTTPClient) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: ClientInfo{
			Name:    "artix",
			Version: "1.0.0",
		},
		Capabilities: map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
	}

	resp, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var res InitializeResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse initialize response: %w", err)
	}
	return &res, nil
}

func (c *HTTPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse tools/list response: %w", err)
	}
	return res.Tools, nil
}

func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	params := ToolCallParams{
		Name:      name,
		Arguments: args,
	}

	resp, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var res ToolCallResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse tools/call response: %w", err)
	}
	return &res, nil
}

func (c *HTTPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	resp, err := c.sendRequest(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Resources []MCPResource `json:"resources"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse resources/list response: %w", err)
	}
	return res.Resources, nil
}

func (c *HTTPClient) ReadResource(ctx context.Context, uri string) (*ResourceContent, error) {
	params := map[string]string{
		"uri": uri,
	}

	resp, err := c.sendRequest(ctx, "resources/read", params)
	if err != nil {
		return nil, err
	}

	var res struct {
		Contents []ResourceContent `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to parse resources/read response: %w", err)
	}
	if len(res.Contents) == 0 {
		return nil, fmt.Errorf("no content found for resource: %s", uri)
	}
	return &res.Contents[0], nil
}

func (c *HTTPClient) Close() error {
	return nil
}
