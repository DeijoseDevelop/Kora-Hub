// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/search"
	"github.com/DeijoseDevelop/Kora-Hub/internal/tasks"
	"github.com/oklog/ulid/v2"
)

// Handler despacha peticiones MCP (JSON-RPC sobre HTTP streamable).
type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Handle procesa una peticion JSON-RPC. userID/wantWorkspace vienen del
// transporte (middleware auth + ?workspace=). Devuelve la respuesta y
// el status HTTP (202 para notificaciones sin id).
func (h *Handler) Handle(ctx context.Context, userID, wantWorkspace string, body []byte) (Response, int) {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" {
		return Response{JSONRPC: "2.0", Error: &RPCError{Code: codeParseError, Message: "JSON-RPC 2.0 invalido"}}, 200
	}
	// notificacion (sin id): nunca responde cuerpo
	if len(req.ID) == 0 {
		return Response{}, 202
	}
	resp := Response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		version := h.deps.Version
		if version == "" {
			version = "dev"
		}
		resp.Result = map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "kora-hub", "version": version},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": tools}
	case "tools/call":
		resp.Result = h.callTool(ctx, userID, wantWorkspace, req.Params)
	default:
		resp.Error = &RPCError{Code: codeMethodNotFound, Message: "metodo no soportado: " + req.Method}
	}
	return resp, 200
}

// callTool ejecuta la tool con sus argumentos; el workspace se resuelve
// por ?workspace=/argumento o el primero del usuario, igual que REST.
func (h *Handler) callTool(ctx context.Context, userID, wantWorkspace string, rawParams json.RawMessage) any {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return errResult("params invalidos")
	}
	var args map[string]any
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return errResult("arguments debe ser un objeto")
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	// el argumento "workspace" de la tool prevalece sobre el query param
	if w, _ := args["workspace"].(string); w != "" {
		wantWorkspace = w
	}

	if params.Name == "list_workspaces" {
		return h.toolListWorkspaces(ctx, userID)
	}

	ws, err := h.deps.Resolve(ctx, wantWorkspace, userID)
	if err != nil {
		return errResult("workspace no resuelto: " + err.Error())
	}

	switch params.Name {
	case "list_docs":
		return h.toolListDocs(ctx, ws)
	case "get_doc":
		return h.toolGetDoc(ctx, ws, args)
	case "create_doc":
		return h.toolCreateDoc(ctx, ws, args)
	case "update_doc":
		return h.toolUpdateDoc(ctx, userID, ws, args)
	case "search":
		return h.toolSearch(ctx, ws, args)
	case "list_tasks":
		return h.toolListTasks(ctx, ws, args)
	case "quick_add_task":
		return h.toolQuickAddTask(ctx, ws, args)
	case "set_task_done":
		return h.toolSetTaskDone(ctx, ws, args)
	default:
		return errResult("tool desconocida: " + params.Name)
	}
}

func strArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func boolArg(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func requireEditor(ws Workspace) *ToolResult {
	if ws.Role == "viewer" {
		res := errResult("rol viewer: solo lectura")
		return &res
	}
	return nil
}

// --------------------------- Implementaciones --------------------------

func (h *Handler) toolListWorkspaces(ctx context.Context, userID string) any {
	rows, err := h.deps.Queries.ListWorkspacesByUser(ctx, userID)
	if err != nil {
		return errResult("error de base de datos")
	}
	return textResult(map[string]any{"workspaces": rows})
}

func (h *Handler) toolListDocs(ctx context.Context, ws Workspace) any {
	rows, err := h.deps.Queries.ListDocsByWorkspace(ctx, ws.ID)
	if err != nil {
		return errResult("error de base de datos")
	}
	type out struct {
		ID        string `json:"id"`
		Path      string `json:"path"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updated_at"`
	}
	docs := make([]out, 0, len(rows))
	for _, d := range rows {
		docs = append(docs, out{ID: d.ID, Path: d.Path, Title: d.Title, UpdatedAt: d.UpdatedAt})
	}
	return textResult(map[string]any{"docs": docs})
}

func (h *Handler) toolGetDoc(ctx context.Context, ws Workspace, args map[string]any) any {
	doc, err := h.deps.Queries.GetDocByID(ctx, db.GetDocByIDParams{ID: strArg(args, "id"), WorkspaceID: ws.ID})
	if err != nil {
		return errResult("documento no encontrado")
	}
	content, err := h.deps.Store.Read(ws.Slug, doc.Path)
	if err != nil {
		return errResult("archivo canonico no disponible")
	}
	return textResult(map[string]any{
		"id": doc.ID, "path": doc.Path, "title": doc.Title,
		"content": string(content), "content_hash": doc.ContentHash,
	})
}

func (h *Handler) toolCreateDoc(ctx context.Context, ws Workspace, args map[string]any) any {
	if r := requireEditor(ws); r != nil {
		return *r
	}
	rel := strArg(args, "path")
	if rel == "" || strArg(args, "title") == "" {
		return errResult("path y title son obligatorios")
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		rel += ".md"
	}
	if err := h.deps.Store.Write(ws.Slug, rel, []byte(strArg(args, "content"))); err != nil {
		return errResult("no se pudo escribir el documento")
	}
	indexed, _ := h.deps.Indexer.ReindexWorkspace(ctx, ws.ID, ws.Slug)
	return textResult(map[string]any{"path": rel, "indexed": indexed})
}

func (h *Handler) toolUpdateDoc(ctx context.Context, userID string, ws Workspace, args map[string]any) any {
	if r := requireEditor(ws); r != nil {
		return *r
	}
	if _, ok := args["content"]; !ok {
		return errResult("content es obligatorio en update_doc")
	}
	doc, err := h.deps.Queries.GetDocByID(ctx, db.GetDocByIDParams{ID: strArg(args, "id"), WorkspaceID: ws.ID})
	if err != nil {
		return errResult("documento no encontrado")
	}
	prev, err := h.deps.Store.Read(ws.Slug, doc.Path)
	if err != nil {
		return errResult("archivo canonico no disponible")
	}
	// snapshot de la version anterior antes de sobrescribir (nunca
	// destructivo — igual que PATCH /docs/:id)
	if vpath, verr := h.deps.Store.WriteVersion(ws.Slug, doc.ID, ulid.Make().String(), prev); verr == nil {
		_ = h.deps.Queries.InsertDocVersion(ctx, db.InsertDocVersionParams{
			DocID: doc.ID, ContentHash: docs.ContentHash(prev),
			CreatedBy:   userID,
			StoragePath: vpath,
		})
	}
	if err := h.deps.Store.Write(ws.Slug, doc.Path, []byte(strArg(args, "content"))); err != nil {
		return errResult("no se pudo escribir el documento")
	}
	h.deps.Indexer.ReindexWorkspace(ctx, ws.ID, ws.Slug) //nolint:errcheck
	return textResult(map[string]any{"id": doc.ID, "path": doc.Path})
}

func (h *Handler) toolSearch(ctx context.Context, ws Workspace, args map[string]any) any {
	q := strArg(args, "q")
	if q == "" {
		return errResult("q es obligatorio")
	}
	tipo := strArg(args, "tipo")
	var out []map[string]string
	if tipo == "" || tipo == "doc" {
		rows, err := search.SearchDocs(ctx, h.deps.Conn, ws.ID, q, 20)
		if err == nil {
			for _, d := range rows {
				out = append(out, map[string]string{"tipo": "doc", "id": d.ID, "title": d.Title, "path": d.Path})
			}
		}
	}
	if tipo == "" || tipo == "tarea" {
		rows, err := h.deps.Queries.SearchTasksByTitle(ctx, db.SearchTasksByTitleParams{
			WorkspaceID: ws.ID, Term: sql.NullString{String: q, Valid: true}, MaxResults: 50,
		})
		if err == nil {
			for _, t := range rows {
				out = append(out, map[string]string{"tipo": "tarea", "id": t.ID, "title": t.Title, "doc_id": t.DocID})
			}
		}
	}
	if tipo == "" || tipo == "adjunto" {
		rows, err := h.deps.Queries.SearchAttachmentsByName(ctx, db.SearchAttachmentsByNameParams{
			WorkspaceID: ws.ID, Term: sql.NullString{String: q, Valid: true}, MaxResults: 50,
		})
		if err == nil {
			for _, a := range rows {
				out = append(out, map[string]string{"tipo": "adjunto", "id": a.ID, "title": a.Filename})
			}
		}
	}
	return textResult(map[string]any{"results": out})
}

func (h *Handler) toolListTasks(ctx context.Context, ws Workspace, args map[string]any) any {
	done := int64(0)
	if boolArg(args, "done") {
		done = 1
	}
	rows, err := h.deps.Queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{
		WorkspaceID: ws.ID, Done: done,
	})
	if err != nil {
		return errResult("error de base de datos")
	}
	return textResult(map[string]any{"tasks": rows})
}

func (h *Handler) toolQuickAddTask(ctx context.Context, ws Workspace, args map[string]any) any {
	if r := requireEditor(ws); r != nil {
		return *r
	}
	text := strArg(args, "text")
	if strings.TrimSpace(text) == "" {
		return errResult("text es obligatorio")
	}
	line := "- [ ] " + strings.TrimSpace(text)
	parsed, ok := tasks.ParseLine(line)
	if !ok {
		return errResult("no se pudo interpretar la tarea")
	}
	canonical := tasks.RoundTrip(parsed, tasks.StateOpen, parsed.DueDate, parsed.Project, parsed.Priority, parsed.Assignee)

	const inboxPath = "inbox.md"
	content, _ := h.deps.Store.Read(ws.Slug, inboxPath)
	var newContent string
	if len(content) == 0 {
		newContent = "# Inbox\n\n" + canonical + "\n"
	} else {
		s := string(content)
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		newContent = s + canonical + "\n"
	}
	if err := h.deps.Store.Write(ws.Slug, inboxPath, []byte(newContent)); err != nil {
		return errResult("no se pudo escribir inbox.md")
	}
	h.deps.Indexer.ReindexWorkspace(ctx, ws.ID, ws.Slug) //nolint:errcheck
	return textResult(map[string]any{"path": inboxPath, "task": parsed.Text})
}

func (h *Handler) toolSetTaskDone(ctx context.Context, ws Workspace, args map[string]any) any {
	if r := requireEditor(ws); r != nil {
		return *r
	}
	task, err := h.deps.Queries.GetTaskByID(ctx, db.GetTaskByIDParams{
		ID: strArg(args, "id"), WorkspaceID: ws.ID,
	})
	if err != nil {
		return errResult("tarea no encontrada")
	}
	done := boolArg(args, "done")
	doc, err := h.deps.Queries.GetDocByID(ctx, db.GetDocByIDParams{ID: task.DocID, WorkspaceID: ws.ID})
	if err != nil {
		return errResult("documento fuente no encontrado")
	}
	content, err := h.deps.Store.Read(ws.Slug, doc.Path)
	if err != nil {
		return errResult("archivo canonico no disponible")
	}
	lines := strings.Split(string(content), "\n")
	idx := int(task.LineNo) - 1
	if idx < 0 || idx >= len(lines) {
		return errResult("la linea fuente ya no existe")
	}
	parsed, ok := tasks.ParseLine(lines[idx])
	if !ok {
		return errResult("la linea ya no es una tarea valida")
	}
	state := tasks.StateOpen
	if done {
		state = tasks.StateDone
	}
	lines[idx] = tasks.RoundTrip(parsed, state, parsed.DueDate, parsed.Project, parsed.Priority, parsed.Assignee)
	// recurrencia (§6.5): al completar se inserta la siguiente ocurrencia
	if done && parsed.Recur != "" && parsed.DueDate != "" {
		if next := tasks.NextOccurrence(parsed.DueDate, parsed.Recur); next != "" {
			lines = append(lines[:idx+1], append([]string{tasks.SpawnRecurring(parsed.RawLine, next)}, lines[idx+1:]...)...)
		}
	}
	if err := h.deps.Store.Write(ws.Slug, doc.Path, []byte(strings.Join(lines, "\n"))); err != nil {
		return errResult("no se pudo escribir el documento")
	}
	h.deps.Indexer.ReindexWorkspace(ctx, ws.ID, ws.Slug) //nolint:errcheck
	return textResult(map[string]any{"id": task.ID, "done": done})
}
