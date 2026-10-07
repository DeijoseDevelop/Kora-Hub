// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/tasks"
)

// quickAddRequest es la entrada de Quick Add Magic (sección 6.4): texto
// en lenguaje natural sin checkbox, p.ej.
//
//	"revisar facturas mañana @zekrost !alta ~deiver +ventas"
type quickAddRequest struct {
	Text string `json:"text" binding:"required"`
}

// InboxPath es el documento canónico donde vive la bandeja de entrada
// del workspace (toda tarea vive en un archivo, invariante P1).
const InboxPath = "inbox.md"

// handleQuickAdd parsea el texto con el mismo parser de tareas
// embebidas y anexa la línea al final de Inbox.md, reindexando el
// workspace. El parser resuelve fechas relativas y normaliza
// prioridades antes de persistir.
func (s *Server) handleQuickAdd(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	var req quickAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	// el texto se parsea idéntico a una línea de documento
	line := "- [ ] " + strings.TrimSpace(req.Text)
	parsed, ok := tasks.ParseLine(line)
	if !ok {
		s.fail(c, http.StatusBadRequest, "unparseable", "no se pudo interpretar la tarea")
		return
	}
	// round-trip: la línea persistida es la canónica (fechas relativas
	// resueltas por el parser)
	canonical := tasks.RoundTrip(parsed, tasks.StateOpen, parsed.DueDate, parsed.Project, parsed.Priority, parsed.Assignee)

	content, err := s.store.Read(ws.slug, InboxPath)
	var base string
	switch {
	case err == nil:
		base = string(content)
		if base != "" && !strings.HasSuffix(base, "\n") {
			base += "\n"
		}
	case errors.Is(err, os.ErrNotExist):
		base = "# Inbox\n\n"
	default:
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer la bandeja")
		return
	}

	if err := s.store.Write(ws.slug, InboxPath, []byte(base+canonical+"\n")); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo guardar la tarea")
		return
	}
	if _, err := s.indexer.ReindexWorkspace(c, ws.id, ws.slug); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo indexar")
		return
	}

	// devuelve la tarea creada desde el índice (doc_id + line_no)
	doc, err := s.queries.GetDocByPath(c, db.GetDocByPathParams{WorkspaceID: ws.id, Path: InboxPath})
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el índice")
		return
	}
	rows, err := s.queries.ListTasksByWorkspace(c, db.ListTasksByWorkspaceParams{WorkspaceID: ws.id, Done: 0})
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo leer el índice")
		return
	}
	var created *db.Task
	for i := range rows {
		if rows[i].DocID == doc.ID && rows[i].Title == parsed.Text {
			t := rows[i]
			created = &t
			break
		}
	}
	if created == nil {
		s.fail(c, http.StatusInternalServerError, "internal", "la tarea no apareció en el índice")
		return
	}
	c.JSON(http.StatusCreated, toTaskDTO(*created))
}
