// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"net/http"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// Share-links públicos (decisión D5, spec §4.2): un token ULID por doc.
// El token ES la capacidad — GET /public/docs/:token no exige sesión.
// Regenerar el share reemplaza el token (revocación implícita del viejo).

// handleCreateShare POST /docs/:id/share — crea o regenera el
// share-link del documento (editor+). Regenerar invalida el token previo.
func (s *Server) handleCreateShare(c *gin.Context) {
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
	token := ulid.Make().String()
	if err := s.queries.UpsertDocShare(c, db.UpsertDocShareParams{
		DocID: doc.ID, Token: token, CreatedBy: userID,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo crear el enlace")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "url": "/p/" + token})
}

// handleGetShare GET /docs/:id/share — estado del share-link (o null).
func (s *Server) handleGetShare(c *gin.Context) {
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
	share, err := s.queries.GetDocShare(c, doc.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"token": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": share.Token, "url": "/p/" + share.Token, "created_at": share.CreatedAt})
}

// handleDeleteShare DELETE /docs/:id/share — revoca el enlace.
func (s *Server) handleDeleteShare(c *gin.Context) {
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
	if err := s.queries.DeleteDocShare(c, doc.ID); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo revocar")
		return
	}
	c.Status(http.StatusNoContent)
}

// handlePublicDoc GET /public/docs/:token — público, sin sesión:
// el token es la capacidad. Devuelve título + Markdown canónico.
func (s *Server) handlePublicDoc(c *gin.Context) {
	row, err := s.queries.GetPublicDocByToken(c, c.Param("token"))
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "enlace no válido o revocado")
		return
	}
	content, err := s.store.Read(row.Slug, row.Path)
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "documento no disponible")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"title": row.Title, "path": row.Path,
		"content": string(content), "updated_at": row.UpdatedAt,
	})
}
