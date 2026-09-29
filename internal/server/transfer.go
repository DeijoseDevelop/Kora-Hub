// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/DeijoseDevelop/Kora-Hub/internal/attachments"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ---------------------------- Import/Export ----------------------------
// Portabilidad estructural (P1): el workspace es un directorio de
// Markdown; exportar es empaquetarlo en ZIP e importar es volcar un
// ZIP (p. ej. un vault de Obsidian) al árbol canónico y reindexar.

// handleExportWorkspace empaqueta el árbol canónico del workspace en un
// ZIP descargable. Cualquier miembro puede exportar (lectura).
func (s *Server) handleExportWorkspace(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok {
		return
	}
	files, err := s.store.List(r.workspace.Slug)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo listar el workspace")
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rel := range files {
		content, err := s.store.Read(r.workspace.Slug, rel)
		if err != nil {
			continue
		}
		w, err := zw.Create(rel)
		if err != nil {
			continue
		}
		_, _ = w.Write(content)
	}
	if err := zw.Close(); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo generar el ZIP")
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="`+r.workspace.Slug+`-export.zip"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// handleImportWorkspace recibe un ZIP (multipart, campo "file") y lo
// vuelca al árbol canónico del workspace:
//   - los .md (incluidos subdirectorios) se escriben tal cual: merge
//     idempotente por ruta — el mismo ZIP reimportado no duplica;
//   - el resto de archivos (imágenes, PDFs de un vault de Obsidian) se
//     registran como adjuntos bajo la clave import/<ruta>, para que el
//     usuario pueda enlazarlos;
//   - entradas ocultas (.obsidian/, .trash/) y rutas maliciosas (..)
//     se descartan.
//
// Al final se reindexa el workspace completo.
func (s *Server) handleImportWorkspace(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok {
		return
	}
	if r.role == "viewer" {
		s.fail(c, http.StatusForbidden, "forbidden", "se requiere rol editor u owner")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.cfg.MaxUploadMB<<20)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", "falta el campo file o excede el límite")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxUploadMB<<20))
	if err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", "no se pudo leer el ZIP")
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", "el archivo no es un ZIP válido")
		return
	}

	imported, skipped, attached := 0, 0, 0
	// una sola carpeta raíz redundante ("MiVault/x.md") se despoja para
	// que el árbol quede en la raíz del workspace
	strip := commonPrefix(zr)
	for _, f := range zr.File {
		rel, ok := sanitizeZipPath(f.Name)
		if !ok {
			skipped++
			continue
		}
		rel = strings.TrimPrefix(rel, strip)
		if rel == "" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			skipped++
			continue
		}
		content, err := io.ReadAll(io.LimitReader(rc, s.cfg.MaxUploadMB<<20))
		rc.Close()
		if err != nil {
			skipped++
			continue
		}
		if strings.HasSuffix(strings.ToLower(rel), ".md") {
			if err := s.store.Write(r.workspace.Slug, rel, content); err != nil {
				skipped++
				continue
			}
			imported++
			continue
		}
		if s.importAttachment(c, r.workspace.ID, r.workspace.Slug, rel, content, int64(f.UncompressedSize64)) {
			attached++
		} else {
			skipped++
		}
	}

	indexed, err := s.indexer.ReindexWorkspace(c, r.workspace.ID, r.workspace.Slug)
	if err != nil {
		s.logger.Warn("reindex tras import", "slug", r.workspace.Slug, "err", err)
	}
	c.JSON(http.StatusOK, gin.H{
		"imported": imported, "attachments": attached,
		"skipped": skipped, "indexed": indexed,
	})
}

// importAttachment registra un binario del ZIP como adjunto del
// workspace; devuelve false si el tipo no está permitido.
func (s *Server) importAttachment(c *gin.Context, workspaceID, slug, rel string, content []byte, size int64) bool {
	head := content
	if len(head) > 512 {
		head = head[:512]
	}
	mimeType := attachments.SniffMIME(head)
	if !attachments.AllowedMIME(mimeType) {
		return false
	}
	id := ulid.Make().String()
	key := slug + "/import/" + id + "-" + path.Base(rel)
	if err := s.attach.Put(c, key, bytes.NewReader(content), size, mimeType); err != nil {
		return false
	}
	if err := s.queries.CreateAttachment(c, db.CreateAttachmentParams{
		ID: id, WorkspaceID: workspaceID, DocID: sql.NullString{},
		Filename: path.Base(rel), Mime: mimeType,
		SizeBytes: size, StorageKey: key,
	}); err != nil {
		_ = s.attach.Delete(c, key)
		return false
	}
	return true
}

// sanitizeZipPath normaliza una entrada del ZIP a una ruta relativa
// segura; rechaza absolutas, escapes y componentes ocultos.
func sanitizeZipPath(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	// un ".." en la ruta original se rechaza antes de Clean: de lo
	// contrario "a/../x.md" colaría como "x.md" en la raíz
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") {
		return "", false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return "", false
		}
	}
	return clean, true
}

// commonPrefix detecta una carpeta raíz única en el ZIP (los gestores
// de archivos suelen empaquetar "MiVault/**") para despojarla.
func commonPrefix(zr *zip.Reader) string {
	var prefix string
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if i := strings.Index(name, "/"); i >= 0 {
			first := name[:i+1]
			if prefix == "" {
				prefix = first
			} else if prefix != first {
				return ""
			}
		} else {
			return ""
		}
	}
	return prefix
}
