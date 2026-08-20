// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration, sourced exclusively from
// environment variables (P3: un solo artefacto, sin archivos externos).
type Config struct {
	PublicURL     string
	BindAddr      string
	JWTSecret     string
	DBPath        string
	Storage       string // "local" | "s3"
	DataDir       string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	MaxUploadMB   int64
	LogLevel      string
}

// Load reads and validates environment variables. Missing mandatory
// variables are hard errors: failing fast beats failing at runtime.
func Load() (*Config, error) {
	cfg := &Config{
		PublicURL:   getEnv("HUB_PUBLIC_URL", "http://localhost:8080"),
		BindAddr:    getEnv("HUB_BIND", ":8080"),
		JWTSecret:   os.Getenv("HUB_JWT_SECRET"),
		DBPath:      getEnv("HUB_DB_PATH", "data/hub.db"),
		Storage:     strings.ToLower(getEnv("HUB_STORAGE", "local")),
		DataDir:     getEnv("HUB_DATA_DIR", "data"),
		AccessTTL:   15 * time.Minute,
		RefreshTTL:  30 * 24 * time.Hour,
		MaxUploadMB: 20,
		LogLevel:    getEnv("HUB_LOG_LEVEL", "info"),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("HUB_JWT_SECRET es obligatoria (AD-07: JWT stateless)")
	}
	if cfg.Storage != "local" && cfg.Storage != "s3" {
		return nil, fmt.Errorf("HUB_STORAGE debe ser 'local' o 's3', got %q", cfg.Storage)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
