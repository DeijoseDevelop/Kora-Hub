// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package server ensambla el router HTTP (Gin) y expone la API REST v1
// (sección 4.2) junto con el frontend embebido (ADR-04).
package server

import (
	"database/sql"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/DeijoseDevelop/Kora-Hub/internal/auth"
	"github.com/DeijoseDevelop/Kora-Hub/internal/config"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
	"github.com/DeijoseDevelop/Kora-Hub/internal/sync"
)

// Version se inyecta en build con -ldflags (GoReleaser).
var Version = "dev"

// Server agrupa las dependencias de la aplicación.
type Server struct {
	cfg        *config.Config
	queries    *db.Queries
	conn       *sql.DB
	logger     *slog.Logger
	authSvc    *auth.Service
	store      *docs.Store
	indexer    *indexer.Indexer
	syncEngine *sync.Engine
}

func New(cfg *config.Config, queries *db.Queries, conn *sql.DB, logger *slog.Logger, authSvc *auth.Service, store *docs.Store, indexer *indexer.Indexer) *Server {
	return &Server{
		cfg: cfg, queries: queries, conn: conn, logger: logger,
		authSvc: authSvc, store: store, indexer: indexer,
		syncEngine: sync.NewEngine(queries, conn, store, indexer),
	}
}

// Router construye el árbol de rutas.
func (s *Server) Router(webFS fs.FS) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": Version})
	})
	r.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": Version})
	})

	api := r.Group("/api/v1")
	{
		api.POST("/auth/register", s.handleRegister)
		api.POST("/auth/login", s.handleLogin)
		api.POST("/auth/refresh", s.handleRefresh)
		api.GET("/auth/status", s.handleAuthStatus)

		authed := api.Group("", s.authMiddleware())
		{
			authed.POST("/auth/logout", s.handleLogout)
			authed.GET("/auth/me", s.handleMe)

			authed.GET("/workspaces", s.handleListWorkspaces)
			authed.POST("/workspaces", s.handleCreateWorkspace)

			authed.GET("/docs", s.handleListDocs)
			authed.POST("/docs", s.handleCreateDoc)
			authed.GET("/docs/:id", s.handleGetDoc)
			authed.PATCH("/docs/:id", s.handlePatchDoc)
			authed.DELETE("/docs/:id", s.handleDeleteDoc)

			authed.GET("/tasks", s.handleListTasks)
			authed.POST("/tasks", s.handleQuickAdd)
			authed.PATCH("/tasks/:id", s.handlePatchTask)

			authed.GET("/search", s.handleSearch)
			authed.GET("/graph", s.handleGraph)

			authed.GET("/sync/changes", s.handleSyncChanges)
			authed.POST("/sync/push", s.handleSyncPush)

			authed.POST("/admin/reindex", s.handleReindex)
		}
	}

	s.mountWeb(r, webFS)
	return r
}

// mountWeb sirve el frontend embebido (SPA en modo hash) si existe dist/.
func (s *Server) mountWeb(r *gin.Engine, webFS fs.FS) {
	if webFS == nil {
		return
	}
	sub, err := fs.Sub(webFS, "dist")
	if err != nil {
		s.logger.Warn("frontend embebido no disponible", "err", err)
		return
	}
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "ruta no existe"}})
			return
		}
		// SPA en modo hash: servir el archivo real o caer a index.html
		if _, err := fs.Stat(sub, strings.TrimPrefix(path, "/")); err != nil {
			c.Request.URL.Path = "/"
		}
		http.FileServer(http.FS(sub)).ServeHTTP(c.Writer, c.Request)
	})
}
