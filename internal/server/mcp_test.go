// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// mcpCall envia una peticion JSON-RPC al endpoint MCP.
func mcpCall(t *testing.T, srv *Server, token, method string, params any) map[string]any {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/mcp", token, body)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: status %d, body %s", method, w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	return resp
}

// mcpTool invoca una tool y devuelve el texto del primer content
// (el JSON del resultado serializado).
func mcpTool(t *testing.T, srv *Server, token, name string, args map[string]any) map[string]any {
	t.Helper()
	resp := mcpCall(t, srv, token, "tools/call", map[string]any{
		"name": name, "arguments": args,
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatalf("tools/call %s: sin result: %v", name, resp)
	}
	if result["isError"] == true {
		content, _ := result["content"].([]any)
		t.Fatalf("tools/call %s devolvio error: %v", name, content)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("tools/call %s: content vacio", name)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("tools/call %s: content no es JSON: %v", name, err)
	}
	return out
}

// TestMCPProtocol cubre el handshake y el listado de tools.
func TestMCPProtocol(t *testing.T) {
	srv, userID := testServer(t)
	token, _ := srv.authSvc.AccessToken(userID, nil)

	init := mcpCall(t, srv, token, "initialize", nil)
	result, _ := init["result"].(map[string]any)
	if result == nil {
		t.Fatalf("initialize sin result: %v", init)
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "kora-hub" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}

	list := mcpCall(t, srv, token, "tools/list", nil)
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 8 {
		t.Fatalf("tools/list devolvio %d tools", len(tools))
	}
}

// TestMCPTools cubre el flujo real: crear doc, buscarlo, quickadd y
// completar la tarea — todo por MCP ejecutando la logica REST interna.
func TestMCPTools(t *testing.T) {
	srv, userID := testServer(t)
	token, _ := srv.authSvc.AccessToken(userID, nil)

	created := mcpTool(t, srv, token, "create_doc", map[string]any{
		"path": "notas/mcp.md", "title": "Doc MCP", "content": "# Doc MCP\n",
	})
	if created["path"] != "notas/mcp.md" {
		t.Fatalf("create_doc: %v", created)
	}

	docs := mcpTool(t, srv, token, "list_docs", nil)
	list, _ := docs["docs"].([]any)
	if len(list) != 1 {
		t.Fatalf("list_docs: %v", docs)
	}
	doc := list[0].(map[string]any)

	got := mcpTool(t, srv, token, "get_doc", map[string]any{"id": doc["id"]})
	if got["content"] != "# Doc MCP\n" {
		t.Fatalf("get_doc content = %v", got["content"])
	}

	search := mcpTool(t, srv, token, "search", map[string]any{"q": "MCP"})
	if results, _ := search["results"].([]any); len(results) == 0 {
		t.Fatalf("search sin resultados: %v", search)
	}

	mcpTool(t, srv, token, "quick_add_task", map[string]any{
		"text": "revisar MCP #manana @kora !alta",
	})
	tasks := mcpTool(t, srv, token, "list_tasks", nil)
	taskList, _ := tasks["tasks"].([]any)
	if len(taskList) != 1 {
		t.Fatalf("list_tasks: %v", tasks)
	}
	task := taskList[0].(map[string]any)

	mcpTool(t, srv, token, "set_task_done", map[string]any{
		"id": task["id"], "done": true,
	})
	tasks = mcpTool(t, srv, token, "list_tasks", map[string]any{"done": true})
	if done, _ := tasks["tasks"].([]any); len(done) != 1 {
		t.Fatalf("tarea no quedo done: %v", tasks)
	}
}

// TestMCPViewerReadOnly: un viewer puede leer pero las tools de
// escritura devuelven isError — mismo enforcement que REST.
func TestMCPViewerReadOnly(t *testing.T) {
	srv, userID := testServer(t)
	wsID := srv.mustFirstWorkspace(context.Background(), userID)
	_, viewerToken := addUser(t, srv, "viewer", wsID)

	resp := mcpCall(t, srv, viewerToken, "tools/call", map[string]any{
		"name": "create_doc", "arguments": map[string]any{"path": "x.md", "title": "x"},
	})
	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("viewer pudo crear doc: %v", resp)
	}
}
