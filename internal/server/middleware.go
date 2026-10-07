// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimit aplica una ventana deslizante en memoria por clave (IP o
// usuario) — sección 11: login/registro por IP, sync push por token.
// Es deliberadamente simple: un proceso, una instancia (P3).
func rateLimit(limit int, window time.Duration, key func(*gin.Context) string) gin.HandlerFunc {
	var mu sync.Mutex
	hits := map[string][]time.Time{}
	return func(c *gin.Context) {
		k := key(c)
		now := time.Now()
		mu.Lock()
		// purga entradas fuera de la ventana (y las de otras claves si el
		// mapa crece: sin esto, rotar X-Forwarded-For / IPs agota memoria)
		if len(hits) > 4096 {
			for kk, vv := range hits {
				kept := vv[:0]
				for _, t := range vv {
					if now.Sub(t) < window {
						kept = append(kept, t)
					}
				}
				if len(kept) == 0 {
					delete(hits, kk)
				} else {
					hits[kk] = kept
				}
			}
		}
		kept := hits[k][:0]
		for _, t := range hits[k] {
			if now.Sub(t) < window {
				kept = append(kept, t)
			}
		}
		hits[k] = kept
		if len(hits[k]) >= limit {
			mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"code": "rate_limited", "message": "demasiadas peticiones, espera un momento"},
			})
			return
		}
		hits[k] = append(hits[k], now)
		mu.Unlock()
		c.Next()
	}
}

func clientIP(c *gin.Context) string { return c.ClientIP() }

// limitBody acota el cuerpo de las peticiones JSON (presupuesto P5:
// <100 MB RAM por instancia — un body ilimitado es OOM garantizado).
func limitBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}

func userKey(c *gin.Context) string {
	if uid := c.GetString("user_id"); uid != "" {
		return uid
	}
	return c.ClientIP()
}

// securityHeaders fija cabeceras defensivas en todas las respuestas
// (sección 11). HSTS solo cuando la petición llega por TLS (o un proxy
// lo declara): anunciarlo en HTTP llano rompería el arranque local.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		// CSP solo para la app embebida; a la API JSON no aplica
		if len(c.Request.URL.Path) < 5 || c.Request.URL.Path[:5] != "/api/" {
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data: blob:; connect-src 'self'; font-src 'self'; "+
					"worker-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'")
		}
		c.Next()
	}
}
