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
	"time"

	"github.com/DeijoseDevelop/Kora-Hub/internal/attachments"
	"github.com/DeijoseDevelop/Kora-Hub/internal/auth"
	"github.com/DeijoseDevelop/Kora-Hub/internal/config"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
	"github.com/DeijoseDevelop/Kora-Hub/internal/mcp"
	"github.com/DeijoseDevelop/Kora-Hub/internal/sync"
	"github.com/gin-gonic/gin"
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
	attach     attachments.Storage
	mcp        *mcp.Handler
}

func New(cfg *config.Config, queries *db.Queries, conn *sql.DB, logger *slog.Logger, authSvc *auth.Service, store *docs.Store, indexer *indexer.Indexer, attach attachments.Storage) *Server {
	s := &Server{
		cfg: cfg, queries: queries, conn: conn, logger: logger,
		authSvc: authSvc, store: store, indexer: indexer,
		syncEngine: sync.NewEngine(queries, conn, store, indexer),
		attach:     attach,
	}
	// MCP: las tools ejecutan la misma logica interna que la REST —
	// nunca una via paralela con menos garantias (P4: API-first).
	s.mcp = mcp.NewHandler(mcp.Deps{
		Queries: s.queries, Conn: s.conn, Store: s.store, Indexer: s.indexer,
		Config: s.cfg, Logger: s.logger,
		Resolve: s.resolveWorkspace, Version: Version,
	})
	return s
}

// Router construye el árbol de rutas.
func (s *Server) Router(webFS fs.FS) *gin.Engine {
	r := gin.New()
	// sin proxies de confianza: ClientIP() es la IP del socket, no la
	// que el cliente declare en X-Forwarded-For (si no, el rate-limit
	// por IP se bypasea rotando la cabecera)
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Logger(), gin.Recovery(), securityHeaders(), limitBody(8<<20))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": Version})
	})
	r.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": Version})
	})

	api := r.Group("/api/v1")
	{
		// auth: 5/min por IP contra fuerza bruta (sección 11)
		api.POST("/auth/register", rateLimit(5, time.Minute, clientIP), s.handleRegister)
		api.POST("/auth/login", rateLimit(5, time.Minute, clientIP), s.handleLogin)
		api.POST("/auth/refresh", rateLimit(30, time.Minute, clientIP), s.handleRefresh)
		api.GET("/auth/status", s.handleAuthStatus)

		// share-links publicos (D5): el token es la capacidad, sin sesion
		api.GET("/public/docs/:token", s.handlePublicDoc)

		authed := api.Group("", s.authMiddleware())
		{
			authed.POST("/auth/logout", s.handleLogout)
			authed.GET("/auth/me", s.handleMe)

			authed.GET("/workspaces", s.handleListWorkspaces)
			authed.POST("/workspaces", s.handleCreateWorkspace)
			authed.GET("/workspaces/:id", s.handleGetWorkspace)
			authed.PATCH("/workspaces/:id", s.handlePatchWorkspace)
			authed.DELETE("/workspaces/:id", s.handleDeleteWorkspace)
			authed.GET("/workspaces/:id/members", s.handleListMembers)
			authed.POST("/workspaces/:id/members", s.handleAddMember)
			authed.PATCH("/workspaces/:id/members/:uid", s.handleUpdateMemberRole)
			authed.DELETE("/workspaces/:id/members/:uid", s.handleRemoveMember)
			authed.GET("/workspaces/:id/export", s.handleExportWorkspace)
			authed.POST("/workspaces/:id/import", s.handleImportWorkspace)

			authed.GET("/views", s.handleListViews)
			authed.POST("/views", s.handleCreateView)
			authed.DELETE("/views/:id", s.handleDeleteView)

			authed.GET("/docs", s.handleListDocs)
			authed.POST("/docs", s.handleCreateDoc)
			authed.GET("/docs/:id", s.handleGetDoc)
			authed.PATCH("/docs/:id", s.handlePatchDoc)
			authed.DELETE("/docs/:id", s.handleDeleteDoc)
			authed.GET("/docs/:id/versions", s.handleListDocVersions)
			authed.GET("/docs/:id/versions/:vid", s.handleGetDocVersion)
			authed.GET("/docs/:id/backlinks", s.handleListDocBacklinks)
			authed.GET("/docs/:id/share", s.handleGetShare)
			authed.POST("/docs/:id/share", s.handleCreateShare)
			authed.DELETE("/docs/:id/share", s.handleDeleteShare)

			authed.GET("/tasks", s.handleListTasks)
			authed.POST("/tasks", s.handleQuickAdd)
			authed.PATCH("/tasks/:id", s.handlePatchTask)

			authed.GET("/search", s.handleSearch)
			authed.GET("/graph", s.handleGraph)

			authed.POST("/attachments", s.handleUploadAttachment)
			authed.GET("/attachments", s.handleListAttachments)
			authed.GET("/attachments/:id", s.handleGetAttachment)
			authed.DELETE("/attachments/:id", s.handleDeleteAttachment)

			authed.GET("/sync/changes", s.handleSyncChanges)
			authed.POST("/sync/push", rateLimit(60, time.Minute, userKey), s.handleSyncPush)

			authed.POST("/admin/reindex", s.handleReindex)

			authed.POST("/mcp", s.handleMCP)
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
