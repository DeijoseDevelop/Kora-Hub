// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLoginRateLimit: más de 5 logins/min por IP -> 429 (sección 11).
// El router se reutiliza entre peticiones: el limiter vive en el
// middleware y se instancia una vez por Router().
func TestLoginRateLimit(t *testing.T) {
	srv, _ := testServer(t)
	r := srv.Router(nil)

	body, _ := mustJSON(t, loginRequest{Email: "a@b.dev", Password: "x"}), any(nil)
	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		last = httptest.NewRecorder()
		r.ServeHTTP(last, req)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("6º login = %d, esperaba 429", last.Code)
	}

	// otra ruta no está limitada por el bucket de login
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status tras rate-limit de login = %d", w.Code)
	}
}

// TestSecurityHeaders: cabeceras defensivas en API y en la app.
func TestSecurityHeaders(t *testing.T) {
	srv, _ := testServer(t)
	r := srv.Router(nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	h := w.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("falta X-Content-Type-Options")
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Error("falta X-Frame-Options")
	}
	if h.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS no debe anunciarse en HTTP llano")
	}

	// HSTS detrás de proxy TLS
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS debe anunciarse tras proxy TLS")
	}

	// CSP solo en la app, no en la API
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Content-Security-Policy") != "" {
		t.Error("CSP no debe aplicarse a la API")
	}
}
