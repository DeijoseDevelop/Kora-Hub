// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package mcp expone la API v1 como herramientas MCP (Model Context
// Protocol) por streamable HTTP: POST /api/v1/mcp con el mismo Bearer
// de siempre. P4 llevado al extremo: todo lo que hace la UI lo puede
// hacer un agente — las tools ejecutan la misma logica interna que los
// handlers REST, nunca una via paralela con menos garantias.
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/DeijoseDevelop/Kora-Hub/internal/config"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
)

// Request JSON-RPC 2.0 segun MCP (streamable HTTP transport).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response JSON-RPC 2.0.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError codigos JSON-RPC estandar de MCP.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// Workspace resuelto para una llamada de tool.
type Workspace struct {
	ID   string
	Slug string
	Role string
}

// ResolveWorkspace decide el tenant de la llamada: el argumento
// "workspace" de la tool (o el ?workspace= de la URL) o el primero del
// usuario. Misma semantica que el middleware REST.
type ResolveWorkspace func(ctx context.Context, want, userID string) (Workspace, error)

// Deps son las dependencias compartidas con el server REST.
type Deps struct {
	Queries *db.Queries
	Conn    *sql.DB // search.SearchDocs opera directo sobre conn (rowid)
	Store   *docs.Store
	Indexer *indexer.Indexer
	Config  *config.Config
	Logger  *slog.Logger
	Resolve ResolveWorkspace
}

// Tool describimos una herramienta con su JSON Schema de entrada.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// ToolResult es el payload MCP estandar: contenido textual (el JSON de
// la respuesta serializado) + marca de error.
type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(v any) ToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return ToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "serializar resultado: " + err.Error()}}}
	}
	return ToolResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}

func errResult(msg string) ToolResult {
	return ToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: msg}}}
}

// tools son las capacidades publicadas — espejo 1:1 de la API REST v1.
var tools = []Tool{
	{
		Name:        "list_workspaces",
		Description: "Lista los workspaces del usuario autenticado con el rol del caller.",
		InputSchema: obj(nil),
	},
	{
		Name:        "list_docs",
		Description: "Lista los documentos del workspace (id, path, titulo, updated_at).",
		InputSchema: obj(nil),
	},
	{
		Name:        "get_doc",
		Description: "Devuelve el contenido Markdown canonico de un documento por id.",
		InputSchema: obj(map[string]prop{
			"id": {Type: "string", Description: "ID del documento"},
		}, "id"),
	},
	{
		Name:        "create_doc",
		Description: "Crea un documento Markdown en el filesystem canonico (editor+).",
		InputSchema: obj(map[string]prop{
			"path":    {Type: "string", Description: "Ruta relativa .md, p.ej. notas/idea.md"},
			"title":   {Type: "string", Description: "Titulo del documento"},
			"content": {Type: "string", Description: "Contenido Markdown inicial"},
		}, "path", "title"),
	},
	{
		Name:        "update_doc",
		Description: "Reemplaza el contenido de un documento; guarda snapshot de la version anterior (editor+).",
		InputSchema: obj(map[string]prop{
			"id":      {Type: "string", Description: "ID del documento"},
			"content": {Type: "string", Description: "Nuevo contenido Markdown completo"},
		}, "id", "content"),
	},
	{
		Name:        "search",
		Description: "Busqueda full-text en documentos, tareas y adjuntos del workspace.",
		InputSchema: obj(map[string]prop{
			"q":    {Type: "string", Description: "Texto a buscar"},
			"tipo": {Type: "string", Description: "doc | tarea | adjunto (opcional)"},
		}, "q"),
	},
	{
		Name:        "list_tasks",
		Description: "Lista tareas embebidas del workspace; done=false (defecto) solo pendientes.",
		InputSchema: obj(map[string]prop{
			"done": {Type: "boolean", Description: "true = completadas"},
		}),
	},
	{
		Name:        "quick_add_task",
		Description: "Crea una tarea en lenguaje natural (misma gramatica que el Quick Add de la UI): 'revisar propuesta #manana @kora !alta ~deiver *every:1w'.",
		InputSchema: obj(map[string]prop{
			"text": {Type: "string", Description: "Texto natural de la tarea con metadatos"},
		}, "text"),
	},
	{
		Name:        "set_task_done",
		Description: "Marca una tarea como hecha o pendiente reescribiendo la linea Markdown fuente (round-trip).",
		InputSchema: obj(map[string]prop{
			"id":   {Type: "string", Description: "ID de la tarea"},
			"done": {Type: "boolean", Description: "true = completar"},
		}, "id", "done"),
	},
}

// prop/obj son helpers compactos para los JSON Schema de entrada.
type prop struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func obj(props map[string]prop, required ...string) any {
	m := map[string]any{"type": "object"}
	if props != nil {
		m["properties"] = props
	}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}
