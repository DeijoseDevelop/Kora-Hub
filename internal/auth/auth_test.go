// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"testing"
	"time"
)

// TestTokenTypSeparation: un refresh token no debe pasar por ParseAccess
// ni un access por ParseRefresh (antes cualquier JWT HMAC válido valía
// como access: ventana de 30 días si el refresh se filtraba).
func TestTokenTypSeparation(t *testing.T) {
	svc := NewService("test-secret-seguro", 15*time.Minute, 30*24*time.Hour)

	access, err := svc.AccessToken("user-1", map[string]string{"ws": "owner"})
	if err != nil {
		t.Fatal(err)
	}
	refresh, jti, err := svc.RefreshToken("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if jti == "" {
		t.Fatal("refresh sin jti")
	}

	// cada token en su tipo
	if _, err := svc.ParseAccess(access); err != nil {
		t.Fatalf("ParseAccess(access) = %v", err)
	}
	if uid, _, err := svc.ParseRefresh(refresh); err != nil || uid != "user-1" {
		t.Fatalf("ParseRefresh(refresh) = %v, %q", err, uid)
	}

	// cruce rechazado
	if _, err := svc.ParseAccess(refresh); err == nil {
		t.Fatal("ParseAccess(refresh) debería fallar")
	}
	if _, _, err := svc.ParseRefresh(access); err == nil {
		t.Fatal("ParseRefresh(access) debería fallar")
	}

	// basura
	if _, err := svc.ParseAccess("no-es-un-jwt"); err == nil {
		t.Fatal("ParseAccess(basura) debería fallar")
	}
}
