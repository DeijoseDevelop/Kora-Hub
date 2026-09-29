// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// TestSearchTipo: ?tipo= filtra por doc/tarea/adjunto (sección 4.2).
func TestSearchTipo(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	doJSON(t, srv, http.MethodPost, "/api/v1/docs", token,
		docRequest{Path: "facturas.md", Title: "Facturas", Content: "# Facturas\n\n- [ ] pagar factura #hoy\n"})

	search := func(q string) struct {
		Results []struct {
			Tipo  string `json:"tipo"`
			Title string `json:"title"`
		} `json:"results"`
	} {
		w := doJSON(t, srv, http.MethodGet, "/api/v1/search?"+q, token, nil)
		var out struct {
			Results []struct {
				Tipo  string `json:"tipo"`
				Title string `json:"title"`
			} `json:"results"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}

	// sin tipo: doc + tarea
	if r := search("q=factura"); len(r.Results) < 2 {
		t.Fatalf("search sin tipo = %+v", r.Results)
	}
	// solo tareas
	r := search("q=factura&tipo=tarea")
	if len(r.Results) != 1 || r.Results[0].Tipo != "tarea" {
		t.Fatalf("tipo=tarea = %+v", r.Results)
	}
	// solo docs
	r = search("q=factura&tipo=doc")
	if len(r.Results) != 1 || r.Results[0].Tipo != "doc" {
		t.Fatalf("tipo=doc = %+v", r.Results)
	}
	// adjuntos: vacío
	r = search("q=factura&tipo=adjunto")
	if len(r.Results) != 0 {
		t.Fatalf("tipo=adjunto = %+v, esperaba []", r.Results)
	}
}

// TestDocsCursorPagination: listado incremental con next_cursor.
func TestDocsCursorPagination(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	for i := 0; i < 5; i++ {
		doJSON(t, srv, http.MethodPost, "/api/v1/docs", token,
			docRequest{Path: fmt.Sprintf("doc%d.md", i), Title: fmt.Sprintf("Doc %d", i), Content: "# x\n"})
	}

	w := doJSON(t, srv, http.MethodGet, "/api/v1/docs?limit=2", token, nil)
	var page struct {
		Docs []struct {
			Path string `json:"path"`
		} `json:"docs"`
		NextCursor string `json:"next_cursor"`
	}
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Docs) != 2 || page.NextCursor == "" {
		t.Fatalf("page1 = %+v", page)
	}

	w = doJSON(t, srv, http.MethodGet, "/api/v1/docs?limit=2&cursor="+url.QueryEscape(page.NextCursor), token, nil)
	var page2 struct {
		Docs []struct {
			Path string `json:"path"`
		} `json:"docs"`
		NextCursor string `json:"next_cursor"`
	}
	json.Unmarshal(w.Body.Bytes(), &page2)
	if len(page2.Docs) != 2 {
		t.Fatalf("page2 = %+v", page2)
	}
	// páginas disjuntas
	if page.Docs[0].Path == page2.Docs[0].Path || page.Docs[1].Path == page2.Docs[1].Path {
		t.Error("páginas con elementos repetidos")
	}
}
