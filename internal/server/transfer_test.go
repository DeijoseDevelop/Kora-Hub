// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// workspaceID devuelve el ID del workspace "ws" del fixture.
func workspaceID(t *testing.T, srv *Server, userID string) string {
	t.Helper()
	rows, err := srv.queries.ListWorkspacesByUser(context.Background(), userID)
	if err != nil || len(rows) == 0 {
		t.Fatal("workspace del fixture no encontrado")
	}
	return rows[0].ID
}

// postZIP sube un ZIP multipart al endpoint de import.
func postZIP(t *testing.T, srv *Server, wsID, token string, files map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "vault.zip")
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(fw)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsID+"/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	return w
}

// TestExportWorkspace comprueba que el ZIP contiene el árbol canónico
// de Markdown y omite los directorios ocultos internos.
func TestExportWorkspace(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)
	wsID := workspaceID(t, srv, ownerID)

	if err := srv.store.Write("ws", "notas/idea.md", []byte("# Idea\n\ncontenido")); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.WriteVersion("ws", "doc1", "v1", []byte("snapshot interno")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsID+"/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("la respuesta no es un ZIP: %v", err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	if !got["notas/idea.md"] {
		t.Fatalf("falta notas/idea.md en el ZIP: %v", got)
	}
	for name := range got {
		if name == ".versions" || bytes.HasPrefix([]byte(name), []byte(".versions/")) {
			t.Fatalf("el export filtró un directorio interno: %s", name)
		}
	}
}

// TestImportWorkspace vuelca un vault ZIP: los .md van al árbol
// canónico reindexado, los binarios a adjuntos y las rutas
// maliciosas/ocultas se descartan.
func TestImportWorkspace(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)
	wsID := workspaceID(t, srv, ownerID)

	png := "\x89PNG\r\n\x1a\n" + string(make([]byte, 16)) // firma PNG mínima
	w := postZIP(t, srv, wsID, token, map[string]string{
		"MiVault/README.md":        "# Importado\n\n- [ ] tarea #2099-01-01\n",
		"MiVault/notas/reunion.md": "# Reunión\n",
		"MiVault/.obsidian/app.md": "# config interna\n",
		"MiVault/../evil.md":       "# malicioso\n",
		"MiVault/img.png":          png,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", w.Code, w.Body.String())
	}
	var res struct {
		Imported    int `json:"imported"`
		Attachments int `json:"attachments"`
		Skipped     int `json:"skipped"`
		Indexed     int `json:"indexed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 {
		t.Fatalf("imported = %d, esperado 2 (README + reunion)", res.Imported)
	}
	if res.Attachments != 1 {
		t.Fatalf("attachments = %d, esperado 1 (img.png)", res.Attachments)
	}
	if res.Indexed != 2 {
		t.Fatalf("indexed = %d, esperado 2", res.Indexed)
	}

	// el árbol canónico recibió los archivos despojando la carpeta raíz
	if _, err := srv.store.Read("ws", "README.md"); err != nil {
		t.Fatal("README.md no quedó en la raíz del workspace")
	}
	if _, err := srv.store.Read("ws", "notas/reunion.md"); err != nil {
		t.Fatal("notas/reunion.md no quedó en el workspace")
	}
	// nada escapó del workspace ni entraron archivos ocultos
	for _, rel := range []string{"../evil.md", "evil.md", ".obsidian/app.md"} {
		if _, err := srv.store.Read("ws", rel); err == nil {
			t.Fatalf("se escribió una ruta prohibida: %s", rel)
		}
	}
	// los docs importados quedaron indexados tras el reindex
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w2, req)
	var list struct {
		Docs []struct {
			Path string `json:"path"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Docs) != 2 {
		t.Fatalf("docs indexados = %d, esperado 2: %v", len(list.Docs), list.Docs)
	}
}

// TestImportForbidden un viewer no puede importar (muta el árbol).
func TestImportForbidden(t *testing.T) {
	srv, ownerID := testServer(t)
	wsID := workspaceID(t, srv, ownerID)
	_, viewerToken := addUser(t, srv, "viewer", wsID)

	w := postZIP(t, srv, wsID, viewerToken, map[string]string{"a.md": "# a\n"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer import status = %d, esperado 403", w.Code)
	}

	// un no-miembro tampoco
	_, outToken := addUser(t, srv, "viewer", "")
	w = postZIP(t, srv, wsID, outToken, map[string]string{"a.md": "# a\n"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("no-miembro import status = %d, esperado 403", w.Code)
	}
}
