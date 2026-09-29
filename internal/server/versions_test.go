// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// TestDocVersionsSnapshot: cada PATCH guarda la versión anterior como
// archivo en .versions/ y es recuperable por API.
func TestDocVersionsSnapshot(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	// crear doc
	w := doJSON(t, srv, http.MethodPost, "/api/v1/docs", token,
		docRequest{Path: "nota.md", Title: "Nota", Content: "# Nota\n\nv1\n"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	docID := created["id"].(string)

	// dos ediciones -> dos versiones
	doJSON(t, srv, http.MethodPatch, "/api/v1/docs/"+docID, token,
		docUpdateRequest{Content: "# Nota\n\nv2\n"})
	doJSON(t, srv, http.MethodPatch, "/api/v1/docs/"+docID, token,
		docUpdateRequest{Content: "# Nota\n\nv3\n"})

	w = doJSON(t, srv, http.MethodGet, "/api/v1/docs/"+docID+"/versions", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("versions = %d: %s", w.Code, w.Body.String())
	}
	var list struct {
		Versions []struct {
			ID          int64  `json:"id"`
			ContentHash string `json:"content_hash"`
			StoragePath string `json:"storage_path"`
		} `json:"versions"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Versions) != 2 {
		t.Fatalf("versions = %d, esperaba 2", len(list.Versions))
	}
	if list.Versions[0].StoragePath == "" {
		t.Error("storage_path vacío")
	}

	// la versión más reciente (v2) debe devolver el contenido anterior
	w = doJSON(t, srv, http.MethodGet, "/api/v1/docs/"+docID+"/versions/"+strconv.FormatInt(list.Versions[0].ID, 10), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get version = %d: %s", w.Code, w.Body.String())
	}
	var vcontent struct {
		Content string `json:"content"`
	}
	json.Unmarshal(w.Body.Bytes(), &vcontent)
	if vcontent.Content != "# Nota\n\nv2\n" {
		t.Errorf("snapshot = %q, esperaba v2", vcontent.Content)
	}
}

// TestDocBacklinks: el endpoint devuelve los docs que enlazan al doc.
func TestDocBacklinks(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	// doc destino + doc origen con [[wikilink]]
	doJSON(t, srv, http.MethodPost, "/api/v1/docs", token,
		docRequest{Path: "destino.md", Title: "Destino", Content: "# Destino\n"})
	w := doJSON(t, srv, http.MethodPost, "/api/v1/docs", token,
		docRequest{Path: "origen.md", Title: "Origen", Content: "# Origen\n\nVer [[destino]].\n"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create origen = %d: %s", w.Code, w.Body.String())
	}

	// resolver el id del doc destino
	w = doJSON(t, srv, http.MethodGet, "/api/v1/docs", token, nil)
	var docsList struct {
		Docs []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"docs"`
	}
	json.Unmarshal(w.Body.Bytes(), &docsList)
	var destID string
	for _, d := range docsList.Docs {
		if d.Path == "destino.md" {
			destID = d.ID
		}
	}
	if destID == "" {
		t.Fatal("destino.md no indexado")
	}

	w = doJSON(t, srv, http.MethodGet, "/api/v1/docs/"+destID+"/backlinks", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("backlinks = %d: %s", w.Code, w.Body.String())
	}
	var bl struct {
		Backlinks []struct {
			ID    string `json:"id"`
			Path  string `json:"path"`
			Title string `json:"title"`
		} `json:"backlinks"`
	}
	json.Unmarshal(w.Body.Bytes(), &bl)
	if len(bl.Backlinks) != 1 || bl.Backlinks[0].Path != "origen.md" {
		t.Fatalf("backlinks = %+v", bl.Backlinks)
	}
}
