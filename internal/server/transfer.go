// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"io"
	"net/http"
	"path"
	"regexp"
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

	notion := isNotionExport(zr)
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
		if notion {
			rel = stripNotionIDs(rel)
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
		lower := strings.ToLower(rel)
		switch {
		case strings.HasSuffix(lower, ".md"):
			if notion {
				content = rewriteNotionLinks(content)
			}
			if err := s.store.Write(r.workspace.Slug, rel, content); err != nil {
				skipped++
				continue
			}
			imported++
		case notion && strings.HasSuffix(lower, ".csv"):
			// las bases de datos de Notion exportan CSV: se convierten
			// a un doc con tabla Markdown (sustituye .csv por .md)
			md, ok := csvToMarkdown(strings.TrimSuffix(path.Base(rel), path.Ext(rel)), content)
			if !ok {
				skipped++
				continue
			}
			docPath := strings.TrimSuffix(rel, path.Ext(rel)) + ".md"
			if err := s.store.Write(r.workspace.Slug, docPath, md); err != nil {
				skipped++
				continue
			}
			imported++
		default:
			if s.importAttachment(c, r.workspace.ID, r.workspace.Slug, rel, content, int64(f.UncompressedSize64)) {
				attached++
			} else {
				skipped++
			}
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

// ------------------------- Normalización Notion -------------------------
// El export de Notion sufija cada página y carpeta con su ID interno
// ("Mi Página 1a2b3c….md"), enlaza entre páginas con esos nombres
// codificados y exporta las bases de datos como CSV.

var notionSuffixRe = regexp.MustCompile(`^(.+?)\s[0-9a-fA-F]{32}$`)

// isNotionExport detecta el export de Notion por el sufijo de 32 hex en
// los nombres de archivo (cualquier entrada lo delata).
func isNotionExport(zr *zip.Reader) bool {
	for _, f := range zr.File {
		base := path.Base(strings.ReplaceAll(f.Name, "\\", "/"))
		if notionSuffixRe.MatchString(strings.TrimSuffix(base, path.Ext(base))) {
			return true
		}
	}
	return false
}

// stripNotionIDs quita el sufijo " <32-hex>" de cada segmento de la
// ruta (tanto carpetas como el propio archivo), conservando extensión.
func stripNotionIDs(rel string) string {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		ext := path.Ext(p)
		if m := notionSuffixRe.FindStringSubmatch(strings.TrimSuffix(p, ext)); m != nil {
			parts[i] = m[1] + ext
		}
	}
	return strings.Join(parts, "/")
}

// notionLinkRe localiza el sufijo de ID dentro de un destino de enlace
// Markdown: "%20<32hex>.md" o " <32hex>.md" y el equivalente para
// carpetas (…hex/). El ID solo aparece como sufijo de Notion, por lo
// que la sustitución global sobre el contenido es segura.
var (
	notionLinkFileRe = regexp.MustCompile(`(?:%20| )[0-9a-fA-F]{32}\.md`)
	notionLinkDirRe  = regexp.MustCompile(`(?:%20| )[0-9a-fA-F]{32}/`)
)

// rewriteNotionLinks actualiza los enlaces internos del documento para
// apuntar a los nombres ya normalizados (sin el ID de página).
func rewriteNotionLinks(content []byte) []byte {
	s := notionLinkFileRe.ReplaceAllString(string(content), ".md")
	s = notionLinkDirRe.ReplaceAllString(s, "/")
	return []byte(s)
}

// csvToMarkdown convierte el CSV de una base de datos de Notion en un
// documento con tabla Markdown. Se capa a 500 filas de datos para no
// inflar el documento con exports enormes.
func csvToMarkdown(title string, raw []byte) ([]byte, bool) {
	rows, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, false
	}
	if len(rows) > 501 {
		rows = rows[:501]
	}
	esc := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "|", "\\|")
	}
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	writeRow := func(row []string) {
		b.WriteString("|")
		for _, c := range row {
			b.WriteString(" " + esc(c) + " |")
		}
		b.WriteByte('\n')
	}
	writeRow(rows[0])
	b.WriteString("|")
	for range rows[0] {
		b.WriteString(" --- |")
	}
	b.WriteByte('\n')
	for _, row := range rows[1:] {
		writeRow(row)
	}
	return []byte(b.String()), true
}
