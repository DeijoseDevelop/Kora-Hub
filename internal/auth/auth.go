// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package auth implementa autenticación JWT stateless con refresh
// rotativo (ADR-07) y contraseñas bcrypt. Access token de 15 minutos;
// refresh token de 30 días, rotativo y almacenado hasheado para
// permitir revocación (sección 4.3).
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost es el costo de hashing de contraseñas (sección 11).
const BcryptCost = 12

var (
	ErrInvalidToken = errors.New("token inválido")
	ErrExpiredToken = errors.New("token expirado")
)

// Claims son los claims del access token: sub + roles por workspace.
// Typ separa access de refresh: un refresh jamás pasa por ParseAccess.
type Claims struct {
	RolesByWorkspace map[string]string `json:"rbw,omitempty"` // workspaceID -> rol
	Typ              string            `json:"typ,omitempty"`
	jwt.RegisteredClaims
}

// refreshClaims es la forma mínima de un refresh token.
type refreshClaims struct {
	Typ string `json:"typ,omitempty"`
	jwt.RegisteredClaims
}

const (
	TypAccess  = "access"
	TypRefresh = "refresh"
)

// Service emite y valida tokens.
type Service struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewService(secret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// HashPassword hashea con bcrypt costo 12.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	return string(b), err
}

// CheckPassword valida una contraseña contra su hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// AccessToken emite un access token con TTL de 15 minutos.
func (s *Service) AccessToken(userID string, roles map[string]string) (string, error) {
	return s.sign(Claims{
		Typ:              TypAccess,
		RolesByWorkspace: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        ulid.Make().String(), // jti: cada emisión es única
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTTL)),
		},
	})
}

// RefreshToken emite un refresh token con jti (id de rotación). La
// expiración se valida en JWT Y en BD (revocación). Devuelve el token y
// su jti para persistir el hash.
func (s *Service) RefreshToken(userID string) (string, string, error) {
	jti := ulid.Make().String()
	tok, err := s.sign(refreshClaims{
		Typ: TypRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.refreshTTL)),
		},
	})
	return tok, jti, err
}

// ParseRefresh valida un refresh token (firma + expiración + typ) y
// devuelve el userID y el jti para la rotación.
func (s *Service) ParseRefresh(raw string) (userID, jti string, err error) {
	claims := &refreshClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !tok.Valid {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.Typ != TypRefresh {
		return "", "", fmt.Errorf("%w: no es un refresh token", ErrInvalidToken)
	}
	return claims.Subject, claims.ID, nil
}

// ParseAccess valida un access token (firma + expiración + typ) y
// devuelve los claims. Un refresh token no pasa esta validación.
func (s *Service) ParseAccess(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.Typ != TypAccess {
		return nil, fmt.Errorf("%w: no es un access token", ErrInvalidToken)
	}
	return claims, nil
}

// HashToken hashea un refresh token antes de almacenarlo (revocación sin
// almacenar el token en claro). SHA-256: los JWT superan los 72 bytes y
// bcrypt los truncaría.
func HashToken(raw string) (string, error) {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:]), nil
}

// CheckTokenHash compara en tiempo constante un token con su hash.
func CheckTokenHash(hash, raw string) bool {
	want, err := hex.DecodeString(hash)
	if err != nil {
		return false
	}
	got := sha256.Sum256([]byte(raw))
	return subtle.ConstantTimeCompare(want, got[:]) == 1
}

func (s *Service) sign(c jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
}
