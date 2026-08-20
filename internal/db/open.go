// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DeijoseDevelop/Kora-Hub/db"
	_ "modernc.org/sqlite"
)

// Open abre (creando si es necesario) la base SQLite índice y aplica
// las migraciones pendientes. P1: la base de datos es índice
// reconstruible; los archivos Markdown son la fuente canónica.
func Open(path string, logger *slog.Logger) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("crear directorio de datos: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1) // un solo escritor; SQLite embebido

	if err := migrate(conn, logger); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migraciones: %w", err)
	}
	return conn, nil
}

// migrate aplica las migraciones SQL embebidas en orden léxico dentro de
// una transacción. forward-only: las migraciones nunca se editan
// (sección 14).
func migrate(conn *sql.DB, logger *slog.Logger) error {
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	entries, err := db.MigrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		var exists int
		if err := conn.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, name).Scan(&exists); err == nil {
			continue // ya aplicada
		}
		body, err := db.MigrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		logger.Info("migración aplicada", "migration", name)
	}
	return nil
}
