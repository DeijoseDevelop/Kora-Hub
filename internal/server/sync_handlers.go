// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/DeijoseDevelop/Kora-Hub/internal/sync"
)

// GET /sync/changes?since=<cursor> — delta de cambios (sección 9.1).
func (s *Server) handleSyncChanges(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	since, _ := strconv.ParseInt(c.DefaultQuery("since", "0"), 10, 64)
	result, err := s.syncEngine.Pull(c, ws.id, since, 500)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el delta")
		return
	}
	c.JSON(http.StatusOK, result)
}

// POST /sync/push — recepción de la cola offline del cliente.
func (s *Server) handleSyncPush(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	var req struct {
		Commands []sync.PushCommand `json:"commands" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	result, err := s.syncEngine.Push(c, ws.id, ws.slug, req.Commands)
	if err != nil {
		s.fail(c, http.StatusConflict, "sync_conflict", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// DELETE /docs/:id — papelera (deleted_at) + cambio en el change_log.
func (s *Server) handleDeleteDoc(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	docID := c.Param("id")
	if err := s.indexer.DeleteDoc(c, ws.id, ws.slug, docID); err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no encontrado")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}
