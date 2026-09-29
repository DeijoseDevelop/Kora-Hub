// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Command hub es el punto de arranque del binario único: config, base
// de datos, migraciones y HTTP (P3). Un solo artefacto, cero
// dependencias externas.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DeijoseDevelop/Kora-Hub/internal/attachments"
	"github.com/DeijoseDevelop/Kora-Hub/internal/auth"
	"github.com/DeijoseDevelop/Kora-Hub/internal/config"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
	"github.com/DeijoseDevelop/Kora-Hub/internal/server"
	"github.com/DeijoseDevelop/Kora-Hub/internal/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuración inválida", "err", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DBPath, logger)
	if err != nil {
		logger.Error("base de datos", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	queries := db.New(conn)
	authSvc := auth.NewService(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	store := docs.NewStore(cfg.DataDir)
	idx := indexer.New(store, queries, conn, logger)

	attach, err := attachments.NewStorage(cfg.Storage, cfg.DataDir, &attachments.S3Config{
		Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		Region: cfg.S3Region, UseSSL: cfg.S3SSL,
	})
	if err != nil {
		logger.Error("backend de adjuntos", "err", err)
		os.Exit(1)
	}
	if s3, ok := attach.(*attachments.S3); ok {
		if err := s3.EnsureBucket(context.Background()); err != nil {
			logger.Error("bucket S3", "err", err)
			os.Exit(1)
		}
	}
	srv := server.New(cfg, queries, conn, logger, authSvc, store, idx, attach)

	httpSrv := &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           srv.Router(web.FS()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("kora hub arrancando", "addr", cfg.BindAddr, "version", server.Version)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("servidor", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("apagado", "err", err)
	}
	logger.Info("apagado limpio")
}
