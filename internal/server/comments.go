// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/gin-gonic/gin"
)

// Comments viven en el filesystem canónico (.comments/<doc-id>.md):
// P1 — los archivos son la verdad, no una tabla aparte. Formato:
//   - 2026-01-15T10:30:00Z|<user_id>|<texto>
type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

const commentsDir = ".comments"

// handleListDocComments lista los comentarios de un documento.
func (s *Server) handleListDocComments(c *gin.Context) {
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
	comments := s.readComments(ws.slug, doc.ID)
	c.JSON(http.StatusOK, gin.H{"comments": comments})
}

// handleAddDocComment añade un comentario a un documento.
func (s *Server) handleAddDocComment(c *gin.Context) {
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
	var req struct {
		Text string `json:"text" binding:"required,max=2000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		s.fail(c, http.StatusBadRequest, "bad_request", "el comentario no puede estar vacío")
		return
	}
	// sanitize: sin saltos de línea (una entrada = una línea)
	text = strings.ReplaceAll(text, "\n", " ")

	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	line := fmt.Sprintf("- %s|%s|%s\n", now, userID, text)

	rel := fmt.Sprintf("%s/%s.md", commentsDir, doc.ID)
	existing, _ := s.store.Read(ws.slug, rel)
	content := string(existing) + line
	if err := s.store.Write(ws.slug, rel, []byte(content)); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo guardar el comentario")
		return
	}

	// nombre del autor para la respuesta
	displayName := userID
	if u, uerr := s.queries.GetUserByID(c, userID); uerr == nil {
		displayName = u.DisplayName
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": fmt.Sprintf("%s-%d", doc.ID, len(existing)),
		"author": displayName, "text": text, "created_at": now,
	})
}

// readComments lee y parsea el archivo de comentarios de un documento.
func (s *Server) readComments(slug, docID string) []Comment {
	rel := fmt.Sprintf("%s/%s.md", commentsDir, docID)
	raw, err := s.store.Read(slug, rel)
	if err != nil {
		return []Comment{}
	}
	out := []Comment{}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		parts := strings.SplitN(line[2:], "|", 3)
		if len(parts) < 3 {
			continue
		}
		out = append(out, Comment{
			ID:        fmt.Sprintf("%s-%d", docID, i),
			Author:    parts[1],
			Text:      parts[2],
			CreatedAt: parts[0],
		})
	}
	return out
}

// ----------------------------- Activity feed -----------------------------

type ActivityEntry struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Op        string `json:"op"`
	DocTitle  string `json:"doc_title"`
	DocPath   string `json:"doc_path"`
	CreatedAt string `json:"created_at"`
}

// handleWorkspaceActivity devuelve los últimos cambios del workspace.
func (s *Server) handleWorkspaceActivity(c *gin.Context) {
	userID := c.GetString("user_id")
	r, ok := s.workspaceFromPath(c, userID)
	if !ok {
		return
	}
	entries := []ActivityEntry{}

	// docs del workspace para resolver títulos
	docs, _ := s.queries.ListDocsByWorkspace(c, r.workspace.ID)
	titleByID := map[string]string{}
	pathByID := map[string]string{}
	for _, d := range docs {
		titleByID[d.ID] = d.Title
		pathByID[d.ID] = d.Path
	}

	// los últimos 50 cambios del change_log via Pull (sin snapshot)
	changes, err := s.syncEngine.Pull(c, r.workspace.ID, 0, 50)
	if err == nil {
		// reversar para más reciente primero
		for i := len(changes.Changes) - 1; i >= 0; i-- {
			ch := changes.Changes[i]
			entries = append(entries, ActivityEntry{
				ID:        fmt.Sprintf("%s-%d", ch.Entity, ch.Seq),
				Entity:    ch.Entity,
				Op:        ch.Op,
				DocTitle:  titleByID[ch.EntityID],
				DocPath:   pathByID[ch.EntityID],
				CreatedAt: ch.CreatedAt,
			})
		}
	}

	// comentarios recientes
	for _, d := range docs {
		for _, cm := range s.readComments(r.workspace.Slug, d.ID) {
			entries = append(entries, ActivityEntry{
				ID:        "comment-" + cm.ID,
				Entity:    "comment",
				Op:        "create",
				DocTitle:  d.Title,
				DocPath:   d.Path,
				CreatedAt: cm.CreatedAt,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"activity": entries})
}
