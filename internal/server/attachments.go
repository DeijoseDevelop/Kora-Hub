// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"path/filepath"

	"github.com/DeijoseDevelop/Kora-Hub/internal/attachments"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ---------------------------- Attachments -----------------------------
// Multipart upload (sección 10): los bytes van al backend (local/S3) y
// los metadatos al índice. El documento referencia el adjunto con un
// enlace Markdown estándar a /api/v1/attachments/<id>.

// handleUploadAttachment recibe multipart/form-data con campo "file" y
// opcionalmente "doc_id" para vincularlo a un documento.
func (s *Server) handleUploadAttachment(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.cfg.MaxUploadMB<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", "falta el campo file o excede el límite")
		return
	}
	defer file.Close()

	// MIME whitelist: se sniffe el contenido real, no el declarado.
	// Los bytes leídos se reinyectan con MultiReader antes del resto.
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mimeType := attachments.SniffMIME(head[:n])
	if !attachments.AllowedMIME(mimeType) {
		s.fail(c, http.StatusUnsupportedMediaType, "mime_not_allowed", "tipo de archivo no permitido: "+mimeType)
		return
	}

	id := ulid.Make().String()
	filename := filepath.Base(header.Filename)
	key := ws.slug + "/" + id + "-" + filename

	if err := s.attach.Put(c, key, io.MultiReader(bytes.NewReader(head[:n]), file), header.Size, mimeType); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo guardar el adjunto")
		return
	}

	var docID sql.NullString
	if d := c.PostForm("doc_id"); d != "" {
		if _, err := s.queries.GetDocByID(c, db.GetDocByIDParams{ID: d, WorkspaceID: ws.id}); err == nil {
			docID = sql.NullString{String: d, Valid: true}
		}
	}
	if err := s.queries.CreateAttachment(c, db.CreateAttachmentParams{
		ID: id, WorkspaceID: ws.id, DocID: docID,
		Filename: filename, Mime: mimeType,
		SizeBytes: header.Size, StorageKey: key,
	}); err != nil {
		_ = s.attach.Delete(c, key)
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo registrar el adjunto")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": id, "filename": filename, "mime": mimeType,
		"size_bytes": header.Size,
		"url":        "/api/v1/attachments/" + id + "?workspace=" + ws.id,
	})
}

func (s *Server) handleListAttachments(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	rows, err := s.queries.ListAttachmentsByWorkspace(c, ws.id)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.Attachment{}
	}
	c.JSON(http.StatusOK, gin.H{"attachments": rows})
}

// handleGetAttachment sirve los bytes con el MIME registrado. Los
// clientes autenticados descargan vía API; para compartir públicamente
// habrá URLs firmadas en una fase posterior.
func (s *Server) handleGetAttachment(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	att, err := s.queries.GetAttachment(c, db.GetAttachmentParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "adjunto no encontrado")
		return
	}
	rc, err := s.attach.Get(c, att.StorageKey)
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "bytes no disponibles")
		return
	}
	defer rc.Close()
	c.Header("Content-Type", att.Mime)
	c.Header("Content-Disposition", "inline; filename=\""+att.Filename+"\"")
	c.Header("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(c.Writer, rc)
}

func (s *Server) handleDeleteAttachment(c *gin.Context) {
	userID := c.GetString("user_id")
	ws, ok := s.workspaceOf(c, userID)
	if !ok {
		return
	}
	if !s.requireEditor(c, ws) {
		return
	}
	att, err := s.queries.GetAttachment(c, db.GetAttachmentParams{ID: c.Param("id"), WorkspaceID: ws.id})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "adjunto no encontrado")
		return
	}
	if err := s.attach.Delete(c, att.StorageKey); err != nil {
		s.logger.Warn("borrar bytes del adjunto", "key", att.StorageKey, "err", err)
	}
	if err := s.queries.DeleteAttachment(c, db.DeleteAttachmentParams{ID: att.ID, WorkspaceID: ws.id}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo eliminar")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}
