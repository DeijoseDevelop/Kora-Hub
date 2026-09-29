// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ---------------------------- Saved views ------------------------------
// D1 (aprobada 2026-09-29): filtros serializados por workspace que la
// UI aplica sobre las proyecciones del indice. Son configuracion del
// workspace (SQLite), no contenido canonico.

// handleListViews devuelve las vistas guardadas del workspace.
func (s *Server) handleListViews(c *gin.Context) {
	ws, ok := s.workspaceOf(c, c.GetString("user_id"))
	if !ok {
		return
	}
	rows, err := s.queries.ListSavedViews(c, ws.id)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.SavedView{}
	}
	c.JSON(http.StatusOK, gin.H{"views": rows})
}

type savedViewRequest struct {
	Name    string          `json:"name" binding:"required,max=120"`
	Filters json.RawMessage `json:"filters" binding:"required"`
}

// handleCreateView guarda una vista con sus filtros (editor+).
// filters llega como JSON libre serializado tal cual — la UI es la
// unica que lo interpreta; el backend solo valida que sea JSON.
func (s *Server) handleCreateView(c *gin.Context) {
	ws, ok := s.workspaceOf(c, c.GetString("user_id"))
	if !ok || !s.requireEditor(c, ws) {
		return
	}
	var req savedViewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	trimmed := bytes.TrimSpace(req.Filters)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) || len(trimmed) > 4096 {
		s.fail(c, http.StatusBadRequest, "bad_request", "filters debe ser un objeto JSON (<=4KB)")
		return
	}
	id := ulid.Make().String()
	if err := s.queries.CreateSavedView(c, db.CreateSavedViewParams{
		ID: id, WorkspaceID: ws.id, Name: req.Name,
		Filters: string(req.Filters), CreatedBy: sql.NullString{String: c.GetString("user_id"), Valid: true},
	}); err != nil {
		s.fail(c, http.StatusConflict, "conflict", "ya existe una vista con ese nombre")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name})
}

// handleDeleteView elimina una vista guardada (editor+).
func (s *Server) handleDeleteView(c *gin.Context) {
	ws, ok := s.workspaceOf(c, c.GetString("user_id"))
	if !ok || !s.requireEditor(c, ws) {
		return
	}
	if _, err := s.queries.GetSavedView(c, db.GetSavedViewParams{
		ID: c.Param("id"), WorkspaceID: ws.id,
	}); err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "vista no encontrada")
		return
	}
	if err := s.queries.DeleteSavedView(c, db.DeleteSavedViewParams{
		ID: c.Param("id"), WorkspaceID: ws.id,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo eliminar")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}
