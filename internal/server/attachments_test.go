// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// uploadTestFile envía un multipart con campo "file".
func uploadTestFile(t *testing.T, srv *Server, token, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(rec, req)
	return rec
}

// TestAttachmentsLifecycle: subir, listar, descargar y borrar.
func TestAttachmentsLifecycle(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	// PNG mínimo (cabecera mágica real para el sniffing)
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 100)...)
	w := uploadTestFile(t, srv, token, "img.png", png)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload = %d: %s", w.Code, w.Body.String())
	}
	var up map[string]any
	json.Unmarshal(w.Body.Bytes(), &up)
	attID := up["id"].(string)
	if up["mime"] != "image/png" {
		t.Errorf("mime = %v", up["mime"])
	}

	// listado
	w = doJSON(t, srv, http.MethodGet, "/api/v1/attachments", token, nil)
	var list struct {
		Attachments []struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
		} `json:"attachments"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Attachments) != 1 || list.Attachments[0].ID != attID {
		t.Fatalf("attachments = %+v", list.Attachments)
	}

	// descarga
	req := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/"+attID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), png) {
		t.Error("contenido descargado no coincide")
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type = %s", rec.Header().Get("Content-Type"))
	}

	// borrado
	w = doJSON(t, srv, http.MethodDelete, "/api/v1/attachments/"+attID, token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/attachments/"+attID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("download tras delete = %d, esperaba 404", rec.Code)
	}
}

// TestAttachmentRejectsExecutable: la whitelist bloquea MIME ejecutables.
func TestAttachmentRejectsExecutable(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	exe := append([]byte("MZ"), bytes.Repeat([]byte{0}, 200)...)
	w := uploadTestFile(t, srv, token, "evil.exe", exe)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("upload exe = %d, esperaba 415", w.Code)
	}
}
