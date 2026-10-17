package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func formatLSPMessage(payload interface{}) []byte {
	b, _ := json.Marshal(payload)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(b))
	return append([]byte(header), b...)
}

func readLSPMessage(r *bufio.Reader) (map[string]interface{}, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				contentLength, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	var res map[string]interface{}
	err := json.Unmarshal(body, &res)
	return res, err
}

func TestLSPServer_FullLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kritix-lsp-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	clientInR, clientInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()
	bufReader := bufio.NewReader(serverOutR)

	server := NewServer(tempDir, clientInR, serverOutW)

	go func() {
		_ = server.Serve()
	}()

	// 1. Initialize
	initReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(initReq))

	resp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read initialize response: %v", err)
	}
	if resp["id"].(float64) != 1 {
		t.Errorf("unexpected id in response: %+v", resp)
	}
	result := resp["result"].(map[string]interface{})
	caps := result["capabilities"].(map[string]interface{})
	if caps["textDocumentSync"].(float64) != 1 {
		t.Errorf("unexpected capabilities: %+v", caps)
	}

	// 2. Open document with Taboo violations
	didOpenReq := LSPRequest{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: json.RawMessage(`{
			"textDocument": {
				"uri": "file:///app/src/Main.kt",
				"languageId": "kotlin",
				"version": 1,
				"text": "import android.database.sqlite.SQLiteDatabase\nfun test() { Thread.sleep(1000) }"
			}
		}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(didOpenReq))

	notif, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read diagnostics notification: %v", err)
	}
	if notif["method"] != "textDocument/publishDiagnostics" {
		t.Errorf("expected publishDiagnostics, got: %+v", notif)
	}
	params := notif["params"].(map[string]interface{})
	diags := params["diagnostics"].([]interface{})
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	d1 := diags[0].(map[string]interface{})
	if d1["code"] != "TABOO-01" {
		t.Errorf("expected TABOO-01, got %v", d1["code"])
	}

	d2 := diags[1].(map[string]interface{})
	if d2["code"] != "TABOO-02" {
		t.Errorf("expected TABOO-02, got %v", d2["code"])
	}

	// 3. Request CodeLens
	lensReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "textDocument/codeLens",
		Params:  json.RawMessage(`{"textDocument": {"uri": "file:///app/src/Main.kt"}}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(lensReq))

	lensResp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read codeLens response: %v", err)
	}
	lenses := lensResp["result"].([]interface{})
	if len(lenses) != 1 {
		t.Fatalf("expected 1 code lens, got %d", len(lenses))
	}

	// 4. Request CodeActions
	actionReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "textDocument/codeAction",
		Params:  json.RawMessage(`{"textDocument": {"uri": "file:///app/src/Main.kt"}}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(actionReq))

	actResp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read codeAction response: %v", err)
	}
	actions := actResp["result"].([]interface{})
	if len(actions) != 2 {
		t.Fatalf("expected 2 code actions, got %d", len(actions))
	}

	// 5. Exit
	exitReq := LSPRequest{
		JSONRPC: "2.0",
		Method:  "exit",
	}
	_, _ = clientInW.Write(formatLSPMessage(exitReq))
	_ = clientInW.Close()
	_ = serverOutW.Close()
}

func TestLSPServer_CancelRequest(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artix-lsp-cancel-*")
	if err != nil {
		t.Fatalf("tempDir failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	clientInR, clientInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()
	bufReader := bufio.NewReader(serverOutR)

	server := NewServer(tempDir, clientInR, serverOutW)
	go func() {
		_ = server.Serve()
	}()

	// Send cancel request for request ID 99
	cancelReq := LSPRequest{
		JSONRPC: "2.0",
		Method:  "$/cancelRequest",
		Params:  json.RawMessage(`{"id": 99}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(cancelReq))

	// Verify server remains healthy and responds to subsequent requests
	initReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      100,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(initReq))

	resp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read initialize response after cancel: %v", err)
	}
	if resp["id"].(float64) != 100 {
		t.Errorf("unexpected id in response: %+v", resp)
	}

	exitReq := LSPRequest{JSONRPC: "2.0", Method: "exit"}
	_, _ = clientInW.Write(formatLSPMessage(exitReq))
	_ = clientInW.Close()
	_ = serverOutW.Close()
}

func TestP2_LSP_ExecuteCommandAndGovernance(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artix-lsp-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	clientInR, clientInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()
	bufReader := bufio.NewReader(serverOutR)

	server := NewServer(tempDir, clientInR, serverOutW)
	go func() { _ = server.Serve() }()

	// 1. Initialize: assert executeCommandProvider advertises commands
	initReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(initReq))
	resp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read initialize response: %v", err)
	}
	result, _ := resp["result"].(map[string]any)
	caps, _ := result["capabilities"].(map[string]any)
	cmdProv, _ := caps["executeCommandProvider"].(map[string]any)
	if cmdProv == nil {
		t.Fatalf("SECURITY/LSP FLAW: executeCommandProvider not advertised in capabilities!")
	}
	cmds, _ := cmdProv["commands"].([]any)
	foundPlan, foundDelib, foundFix := false, false, false
	for _, c := range cmds {
		switch c {
		case "artix.plan":
			foundPlan = true
		case "artix.deliberate":
			foundDelib = true
		case "artix.fixTaboo":
			foundFix = true
		}
	}
	if !foundPlan || !foundDelib || !foundFix {
		t.Fatalf("missing advertised commands in capabilities: %v", cmds)
	}

	// 2. Execute command artix.deliberate through the gate
	execReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "workspace/executeCommand",
		Params:  json.RawMessage(`{"command": "artix.deliberate", "arguments": []}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(execReq))
	execResp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read executeCommand response: %v", err)
	}
	execRes, _ := execResp["result"].(map[string]any)
	if execRes == nil || execRes["ok"] != true || execRes["governed"] != true {
		t.Fatalf("expected governed execution result, got: %+v", execResp)
	}

	// 3. Unknown command returns -32601
	unknownReq := LSPRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "workspace/executeCommand",
		Params:  json.RawMessage(`{"command": "artix.unknown", "arguments": []}`),
	}
	_, _ = clientInW.Write(formatLSPMessage(unknownReq))
	unkResp, err := readLSPMessage(bufReader)
	if err != nil {
		t.Fatalf("failed to read unknown command response: %v", err)
	}
	errObj, _ := unkResp["error"].(map[string]any)
	if errObj == nil || errObj["code"].(float64) != -32601 {
		t.Fatalf("expected -32601 error for unknown command, got: %+v", unkResp)
	}

	_ = clientInW.Close()
	_ = serverOutW.Close()
}

