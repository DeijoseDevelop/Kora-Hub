// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DeijoseDevelop/Kora-Hub/internal/auth"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/search"
	"github.com/DeijoseDevelop/Kora-Hub/internal/tasks"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// -------------------------------- Auth --------------------------------

type registerRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	DisplayName string `json:"display_name" binding:"required"`
}

func (s *Server) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "bad_request", "message": err.Error()}})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo hashear la contraseña")
		return
	}
	userID := ulid.Make().String()
	if err := s.queries.CreateUser(c, db.CreateUserParams{
		ID:           userID,
		Email:        req.Email,
		PasswordHash: hash,
		DisplayName:  req.DisplayName,
	}); err != nil {
		s.fail(c, http.StatusConflict, "email_exists", "ese email ya está registrado")
		return
	}

	// onboarding: workspace "Personal" auto-creado (tipo Notion) para que
	// el primer doc/tarea funcionen sin pasos intermedios.
	ws := s.createPersonalWorkspace(c, userID)
	if ws != nil {
		c.Set("personal_workspace", ws)
	}
	s.issueTokens(c, userID, nil)
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	user, err := s.queries.GetUserByEmail(c, req.Email)
	if err == sql.ErrNoRows || !auth.CheckPassword(user.PasswordHash, req.Password) {
		s.fail(c, http.StatusUnauthorized, "invalid_credentials", "credenciales inválidas")
		return
	}
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	// roles por workspace (sección 4.3: la UI solo refleja, el backend garantiza)
	s.issueTokens(c, user.ID, nil)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// handleRefresh rota el refresh token: valida firma + BD (no revocado,
// no expirado), revoca el viejo y emite un par nuevo. Público: el
// access token puede estar expirado (esa es la razón de refrescar).
func (s *Server) handleRefresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	userID, jti, err := s.authSvc.ParseRefresh(req.RefreshToken)
	if err != nil {
		s.fail(c, http.StatusUnauthorized, "invalid_token", "token de refresco inválido")
		return
	}

	row, err := s.queries.GetRefreshTokenByID(c, jti)
	if err != nil || row.RevokedAt.Valid || !auth.CheckTokenHash(row.TokenHash, req.RefreshToken) {
		s.fail(c, http.StatusUnauthorized, "invalid_token", "token de refresco revocado o inexistente")
		return
	}
	if row.ExpiresAt < timeNowSQL() {
		s.fail(c, http.StatusUnauthorized, "invalid_token", "token de refresco expirado")
		return
	}

	// rotación atómica: el viejo queda revocado; un segundo refresh con
	// el mismo token recibe 401 (y el cliente reintenta con el nuevo).
	if err := s.queries.RevokeRefreshToken(c, jti); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo rotar el token")
		return
	}
	go func() { _ = s.queries.DeleteExpiredRefreshTokens(context.Background()) }()

	roles := map[string]string{} // fase MVP: roles por workspace se resuelven en auth/me
	s.issueTokens(c, userID, roles)
}

func (s *Server) handleLogout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
		_, jti, err := s.authSvc.ParseRefresh(req.RefreshToken)
		if err == nil {
			_ = s.queries.RevokeRefreshToken(c, jti)
		}
	}
	c.JSON(http.StatusNoContent, nil)
}

func (s *Server) issueTokens(c *gin.Context, userID string, roles map[string]string) {
	access, err := s.authSvc.AccessToken(userID, roles)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo firmar el token")
		return
	}
	refresh, jti, err := s.authSvc.RefreshToken(userID)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo firmar el refresh")
		return
	}
	hash, err := auth.HashToken(refresh)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo hashear el refresh")
		return
	}
	if err := s.queries.CreateRefreshToken(c, db.CreateRefreshTokenParams{
		ID: jti, UserID: userID, TokenHash: hash,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTTL).Format("2006-01-02 15:04:05"),
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo persistir el refresh")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    int(s.cfg.AccessTTL.Seconds()),
	})
}

// handleMe devuelve el usuario autenticado (para Ajustes).
func (s *Server) handleMe(c *gin.Context) {
	userID := c.GetString("user_id")
	user, err := s.queries.GetUserByID(c, userID)
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "usuario no encontrado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "email": user.Email, "display_name": user.DisplayName})
}

// handleAuthStatus expone si existen usuarios (primer arranque de
// desktop/Docker: la UI muestra "Crear cuenta" por defecto).
func (s *Server) handleAuthStatus(c *gin.Context) {
	n, err := s.queries.CountUsers(c)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"has_users": n > 0})
}

// ----------------------------- Workspaces -----------------------------

type workspaceRequest struct {
	Slug string `json:"slug" binding:"required,lowercase,max=63"`
	Name string `json:"name" binding:"required,max=120"`
}

func (s *Server) handleCreateWorkspace(c *gin.Context) {
	userID := c.GetString("user_id")
	var req workspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	id := ulid.Make().String()
	if err := s.queries.CreateWorkspace(c, db.CreateWorkspaceParams{
		ID: id, Slug: req.Slug, Name: req.Name, OwnerID: userID,
	}); err != nil {
		s.fail(c, http.StatusConflict, "slug_exists", "ese slug ya existe")
		return
	}
	if err := s.queries.AddMembership(c, db.AddMembershipParams{
		UserID: userID, WorkspaceID: id, Role: "owner",
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo crear la membresía")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "slug": req.Slug, "name": req.Name, "owner_id": userID})
}

func (s *Server) handleListWorkspaces(c *gin.Context) {
	userID := c.GetString("user_id")
	rows, err := s.queries.ListWorkspacesByUser(c, userID)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.ListWorkspacesByUserRow{}
	}
	c.JSON(http.StatusOK, gin.H{"workspaces": rows})
}

// ------------------------------- Docs --------------------------------
// P1: la escritura va al filesystem canónico y el índice se regenera.

type docRequest struct {
	Path    string `json:"path" binding:"required,max=255"`
	Title   string `json:"title" binding:"required,max=255"`
	Content string `json:"content"`
}

func (s *Server) handleCreateDoc(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	var req docRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !strings.HasSuffix(strings.ToLower(req.Path), ".md") {
		req.Path += ".md"
	}
	// P1: el handler solo escribe el archivo canónico; el indexer es el
	// único dueño del índice (tareas, FTS, backlinks).
	if err := s.store.Write(ws.slug, req.Path, []byte(req.Content)); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo escribir el documento")
		return
	}
	if _, err := s.indexer.ReindexWorkspace(c, ws.id, ws.slug); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo indexar el documento")
		return
	}
	doc, err := s.queries.GetDocByPath(c, db.GetDocByPathParams{WorkspaceID: ws.id, Path: req.Path})
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el índice")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": doc.ID, "title": doc.Title, "content_hash": doc.ContentHash})
}

// handleListDocs soporta paginación por cursor (sección 4.2):
// ?cursor=<updated_at>|<id> + ?limit=N → next_cursor en la respuesta.
// Sin parámetros devuelve la lista completa (compatibilidad).
func (s *Server) handleListDocs(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	cursor := c.Query("cursor")
	if limit <= 0 && cursor == "" {
		rows, err := s.queries.ListDocsByWorkspace(c, ws.id)
		if err != nil {
			s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
			return
		}
		if rows == nil {
			rows = []db.Doc{}
		}
		c.JSON(http.StatusOK, gin.H{"docs": rows})
		return
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	afterTS := "9999-12-31 23:59:59"
	var afterRowid int64 = 1 << 62 // cursor vacío = primera página
	if cursor != "" {
		parts := strings.SplitN(cursor, "|", 2)
		if len(parts) != 2 {
			s.fail(c, http.StatusBadRequest, "bad_request", "cursor inválido")
			return
		}
		afterTS = parts[0]
		if rid, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			afterRowid = rid
		}
	}
	// rowid es monótono por inserción: cursor estable incluso con
	// updated_at idénticos (sqlc no expone columnas implícitas: SQL directo)
	rows, err := s.conn.QueryContext(c, `
		SELECT rowid AS cursor_id, id, path, title, updated_at
		FROM docs
		WHERE workspace_id = ? AND deleted_at IS NULL
		  AND (updated_at < ? OR (updated_at = ? AND rowid < ?))
		ORDER BY updated_at DESC, rowid DESC
		LIMIT ?`, ws.id, afterTS, afterTS, afterRowid, int64(limit)+1)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	type docRow struct {
		CursorID  int64  `json:"cursor_id"`
		ID        string `json:"id"`
		Path      string `json:"path"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updated_at"`
	}
	out := []docRow{}
	for rows.Next() {
		var d docRow
		if err := rows.Scan(&d.CursorID, &d.ID, &d.Path, &d.Title, &d.UpdatedAt); err != nil {
			rows.Close()
			s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
			return
		}
		out = append(out, d)
	}
	rows.Close()
	resp := gin.H{"docs": out}
	if len(out) > limit {
		last := out[limit-1]
		resp["next_cursor"] = last.UpdatedAt + "|" + strconv.FormatInt(last.CursorID, 10)
		resp["docs"] = out[:limit]
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleGetDoc(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	doc, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no encontrado")
		return
	}
	content, err := s.store.Read(ws.slug, doc.Path)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el archivo canónico")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": doc.ID, "title": doc.Title, "path": doc.Path,
		"content": string(content), "content_hash": doc.ContentHash,
	})
}

type docUpdateRequest struct {
	Content string `json:"content" binding:"required"`
}

func (s *Server) handlePatchDoc(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	doc, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no encontrado")
		return
	}
	var req docUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// snapshot de la versión anterior en .versions/ (P1: los archivos
	// son la verdad; doc_versions solo indexa los snapshots).
	if prev, err := s.store.Read(ws.slug, doc.Path); err == nil && string(prev) != req.Content {
		vpath, verr := s.store.WriteVersion(ws.slug, doc.ID, ulid.Make().String(), prev)
		if verr == nil {
			_ = s.queries.InsertDocVersion(c, db.InsertDocVersionParams{
				DocID: doc.ID, ContentHash: doc.ContentHash,
				CreatedBy: userID, StoragePath: vpath,
			})
		}
	}
	if err := s.store.Write(ws.slug, doc.Path, []byte(req.Content)); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo escribir el documento")
		return
	}
	// el indexer regenera tareas/FTS y actualiza título + hash
	if _, err := s.indexer.ReindexWorkspace(c, ws.id, ws.slug); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo reindexar el documento")
		return
	}
	updated, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: doc.ID, WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el índice")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": updated.ID, "content_hash": updated.ContentHash})
}

// ---------------------------- Doc versions -----------------------------

// handleListDocVersions lista los snapshots de un documento (más
// reciente primero). Cualquier miembro del workspace puede leerlos.
func (s *Server) handleListDocVersions(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	doc, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no encontrado")
		return
	}
	limit := 50
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	rows, err := s.queries.GetDocVersions(c, db.GetDocVersionsParams{DocID: doc.ID, Limit: int64(limit)})
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.DocVersion{}
	}
	c.JSON(http.StatusOK, gin.H{"versions": rows})
}

// handleGetDocVersion devuelve el contenido de un snapshot para el
// diff (la UI calcula las diferencias contra el contenido actual).
func (s *Server) handleGetDocVersion(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	doc, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no encontrado")
		return
	}
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", "id de versión inválido")
		return
	}
	v, err := s.queries.GetDocVersionByID(c, vid)
	if err != nil || v.DocID != doc.ID {
		s.fail(c, http.StatusNotFound, "not_found", "versión no encontrada")
		return
	}
	content, err := s.store.ReadVersion(ws.slug, v.StoragePath)
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "snapshot no disponible")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": v.ID, "doc_id": doc.ID, "content": string(content),
		"content_hash": v.ContentHash, "created_at": v.CreatedAt,
	})
}

// handleListDocBacklinks devuelve los documentos que enlazan a este
// (la tabla backlinks la puebla el indexer a partir de [[wikilinks]]).
func (s *Server) handleListDocBacklinks(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	doc, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no encontrado")
		return
	}
	rows, err := s.queries.ListBacklinksTo(c, db.ListBacklinksToParams{ID: doc.ID, WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.ListBacklinksToRow{}
	}
	c.JSON(http.StatusOK, gin.H{"backlinks": rows})
}

// ------------------------------- Tasks --------------------------------
// Las tareas son proyecciones del índice; PATCH reescribe la línea en
// el Markdown fuente (round-trip, sección 6.2) y reindexa.

// TaskDTO es la representación JSON pública de una tarea: el backend
// serializa los campos nullable como string|null (nunca como el objeto
// interno sql.NullString).
type TaskDTO struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	DocID       string  `json:"doc_id"`
	LineNo      int64   `json:"line_no"`
	Title       string  `json:"title"`
	DueDate     *string `json:"due_date"`
	Project     *string `json:"project"`
	Priority    *string `json:"priority"`
	Assignee    *string `json:"assignee"`
	Done        int64   `json:"done"`
	InProgress  int64   `json:"in_progress"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func toTaskDTO(t db.Task) TaskDTO {
	return TaskDTO{
		ID: t.ID, WorkspaceID: t.WorkspaceID, DocID: t.DocID,
		LineNo: t.LineNo, Title: t.Title,
		DueDate:    nsPtr(t.DueDate),
		Project:    nsPtr(t.Project),
		Priority:   nsPtr(t.Priority),
		Assignee:   nsPtr(t.Assignee),
		Done:       t.Done,
		InProgress: t.InProgress,
		CreatedAt:  t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func toTaskDTOs(rows []db.Task) []TaskDTO {
	out := make([]TaskDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTaskDTO(r))
	}
	return out
}

func nsPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// handleListTasks devuelve una proyección del índice (sección 6.3).
// Las vistas nunca almacenan estado: son consultas materializadas.
//
//	vista=kanban (default) | tabla | calendario | mis-tareas-hoy
func (s *Server) handleListTasks(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}

	var (
		rows []db.Task
		err  error
	)
	switch c.DefaultQuery("vista", "kanban") {
	case "tabla":
		if proyecto := c.Query("proyecto"); proyecto != "" {
			rows, err = s.queries.ListTasksByProject(c, db.ListTasksByProjectParams{
				WorkspaceID: ws.id, Project: sql.NullString{String: proyecto, Valid: true},
			})
		} else {
			rows, err = s.queries.ListTasksByWorkspace(c, db.ListTasksByWorkspaceParams{
				WorkspaceID: ws.id, Done: boolToInt64(c.DefaultQuery("done", "0") == "1"),
			})
		}
	case "calendario":
		rows, err = s.queries.ListTasksByDateRange(c, db.ListTasksByDateRangeParams{
			WorkspaceID: ws.id,
			DueDate:     sql.NullString{String: c.DefaultQuery("desde", "2000-01-01"), Valid: true},
			DueDate_2:   sql.NullString{String: c.DefaultQuery("hasta", "2999-12-31"), Valid: true},
		})
	case "mis-tareas-hoy":
		assignee := c.DefaultQuery("asignado", userID)
		rows, err = s.queries.ListTasksMineToday(c, db.ListTasksMineTodayParams{
			WorkspaceID: ws.id, Assignee: sql.NullString{String: assignee, Valid: true},
		})
	default: // kanban
		done := c.DefaultQuery("done", "0") == "1"
		rows, err = s.queries.ListTasksByWorkspace(c, db.ListTasksByWorkspaceParams{
			WorkspaceID: ws.id, Done: boolToInt64(done),
		})
	}
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.Task{}
	}
	c.JSON(http.StatusOK, gin.H{"tasks": toTaskDTOs(rows)})
}

type patchTaskRequest struct {
	Done     bool   `json:"done"`
	DueDate  string `json:"due_date,omitempty"`
	Project  string `json:"project,omitempty"`
	Priority string `json:"priority,omitempty"`
	Assignee string `json:"assignee,omitempty"`
}

func (s *Server) handlePatchTask(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	task, err := s.queries.GetTaskByID(c, db.GetTaskByIDParams{
		ID: c.Param("id"), WorkspaceID: ws.id,
	})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "tarea no encontrada")
		return
	}
	var req patchTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	doc, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: task.DocID, WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento fuente no encontrado")
		return
	}
	content, err := s.store.Read(ws.slug, doc.Path)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el archivo canónico")
		return
	}

	lines := strings.Split(string(content), "\n")
	idx := int(task.LineNo) - 1
	if idx < 0 || idx >= len(lines) {
		s.fail(c, http.StatusConflict, "line_missing", "la línea fuente ya no existe")
		return
	}

	// round-trip: reescribir solo la línea original, preservando el resto
	parsed, ok := tasks.ParseLine(lines[idx])
	if ok {
		lines[idx] = tasks.RoundTrip(parsed, req.Done,
			req.DueDate, req.Project, req.Priority, req.Assignee)
	} else {
		// drift: reconstruir la línea mínima sin perder el título
		state := " "
		if req.Done {
			state = "x"
		}
		lines[idx] = "- [" + state + "] " + task.Title
	}

	if err := s.store.Write(ws.slug, doc.Path, []byte(strings.Join(lines, "\n"))); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo escribir el documento")
		return
	}
	if _, err := s.indexer.ReindexWorkspace(c, ws.id, ws.slug); err != nil {
		s.logger.Warn("reindex tras PATCH task", "err", err)
	}

	done := task.Done
	if req.Done {
		done = 1
	} else if !req.Done && task.Done == 1 {
		done = 0
	}
	if err := s.queries.SetTaskDone(c, db.SetTaskDoneParams{
		Done: done, ID: task.ID, WorkspaceID: ws.id,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo actualizar la tarea")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": task.ID, "done": done == 1})
}

// ------------------------------- Search --------------------------------

// SearchResult unifica los tipos buscables (sección 4.2: ?tipo=).
type SearchResult struct {
	Tipo  string `json:"tipo"` // doc | tarea | adjunto
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path,omitempty"`
	DocID string `json:"doc_id,omitempty"`
}

// handleSearch filtra por ?tipo=doc|tarea|adjunto; sin tipo busca en
// todo (docs por FTS5, tareas y adjuntos por LIKE).
func (s *Server) handleSearch(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	q := c.Query("q")
	tipo := c.DefaultQuery("tipo", "")
	limit := 20
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	out := []SearchResult{}

	if tipo == "" || tipo == "doc" {
		docs, err := search.SearchDocs(c, s.conn, ws.id, q, limit)
		if err != nil {
			s.fail(c, http.StatusInternalServerError, "internal", "error de búsqueda")
			return
		}
		for _, d := range docs {
			out = append(out, SearchResult{Tipo: "doc", ID: d.ID, Title: d.Title, Path: d.Path})
		}
	}
	if tipo == "" || tipo == "tarea" {
		rows, err := s.queries.SearchTasksByTitle(c, db.SearchTasksByTitleParams{
			WorkspaceID: ws.id, Term: sql.NullString{String: q, Valid: true}, MaxResults: int64(limit),
		})
		if err != nil {
			s.fail(c, http.StatusInternalServerError, "internal", "error de búsqueda")
			return
		}
		for _, t := range rows {
			out = append(out, SearchResult{Tipo: "tarea", ID: t.ID, Title: t.Title, DocID: t.DocID})
		}
	}
	if tipo == "" || tipo == "adjunto" {
		rows, err := s.queries.SearchAttachmentsByName(c, db.SearchAttachmentsByNameParams{
			WorkspaceID: ws.id, Term: sql.NullString{String: q, Valid: true}, MaxResults: int64(limit),
		})
		if err != nil {
			s.fail(c, http.StatusInternalServerError, "internal", "error de búsqueda")
			return
		}
		for _, a := range rows {
			out = append(out, SearchResult{Tipo: "adjunto", ID: a.ID, Title: a.Filename})
		}
	}
	c.JSON(http.StatusOK, gin.H{"results": out})
}

// ----------------------------- Reindex (admin) --------------------------

func (s *Server) handleReindex(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireOwner(c, ws) {
		return
	}
	n, err := s.indexer.ReindexWorkspace(c, ws.id, ws.slug)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "reindex fallido")
		return
	}
	c.JSON(http.StatusOK, gin.H{"reindexed": n, "workspace": ws.slug})
}

// ------------------------------ Helpers -------------------------------

// authMiddleware valida el Bearer token (sección 4.3): la UI solo
// refleja permisos, el middleware garantiza membresía/rol.
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if len(raw) < 8 || raw[:7] != "Bearer " {
			s.fail(c, http.StatusUnauthorized, "unauthorized", "token requerido")
			c.Abort()
			return
		}
		claims, err := s.authSvc.ParseAccess(raw[7:])
		if err != nil {
			s.fail(c, http.StatusUnauthorized, "unauthorized", "token inválido")
			c.Abort()
			return
		}
		c.Set("user_id", claims.Subject)
		c.Set("roles", claims.RolesByWorkspace)
		c.Next()
	}
}

func (s *Server) fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

type workspaceCtx struct {
	id   string
	slug string
	role string // owner | editor | viewer
}

// workspaceOf resuelve el workspace del request (?workspace= o el
// primero del usuario en fase MVP) y valida la membresía. La autorización
// por rol se aplica después con requireEditor/requireOwner (sección 4.3:
// la UI solo refleja permisos, el backend los garantiza).
func (s *Server) workspaceOf(c *gin.Context, userID string) (workspaceCtx, bool) {
	want := c.Query("workspace")
	if want == "" {
		rows, err := s.queries.ListWorkspacesByUser(c, userID)
		if err != nil || len(rows) == 0 {
			s.fail(c, http.StatusBadRequest, "no_workspace", "crea o selecciona un workspace")
			return workspaceCtx{}, false
		}
		return workspaceCtx{id: rows[0].ID, slug: rows[0].Slug, role: rows[0].Role}, true
	}
	member, err := s.queries.GetMembership(c, db.GetMembershipParams{UserID: userID, WorkspaceID: want})
	if err != nil {
		s.fail(c, http.StatusForbidden, "forbidden", "no perteneces a ese workspace")
		return workspaceCtx{}, false
	}
	ws, err := s.queries.GetWorkspaceByID(c, want)
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "workspace no encontrado")
		return workspaceCtx{}, false
	}
	return workspaceCtx{id: ws.ID, slug: ws.Slug, role: member.Role}, true
}

// requireEditor exige rol owner/editor para mutaciones.
func (s *Server) requireEditor(c *gin.Context, ws workspaceCtx) bool {
	if ws.role == "viewer" {
		s.fail(c, http.StatusForbidden, "forbidden", "rol viewer: solo lectura")
		return false
	}
	return true
}

// requireOwner exige el rol owner (p.ej. reindex).
func (s *Server) requireOwner(c *gin.Context, ws workspaceCtx) bool {
	if ws.role != "owner" {
		s.fail(c, http.StatusForbidden, "forbidden", "se requiere rol owner")
		return false
	}
	return true
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// createPersonalWorkspace crea (o reutiliza) el workspace "Personal" del
// usuario. Devuelve nil si falla (el registro no debe romperse por esto).
func (s *Server) createPersonalWorkspace(c *gin.Context, userID string) *workspaceCtx {
	slug := "personal"
	for attempt := 0; attempt < 3; attempt++ {
		id := ulid.Make().String()
		err := s.queries.CreateWorkspace(c, db.CreateWorkspaceParams{
			ID: id, Slug: slug, Name: "Personal", OwnerID: userID,
		})
		if err == nil {
			if err := s.queries.AddMembership(c, db.AddMembershipParams{
				UserID: userID, WorkspaceID: id, Role: "owner",
			}); err == nil {
				return &workspaceCtx{id: id, slug: slug, role: "owner"}
			}
			return nil
		}
		slug = "personal-" + ulid.Make().String()[:8]
	}
	return nil
}

func timeNowSQL() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
