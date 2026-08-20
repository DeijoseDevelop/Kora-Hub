// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package docs implementa el CRUD de documentos sobre el filesystem
// canónico (P1): los archivos Markdown son la única fuente de verdad;
// la base de datos es índice reconstruible.
package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store es un almacén de documentos sobre un directorio canónico.
// Layout (sección 5.2):
//
//	data/workspaces/<slug>/<proyecto>/README.md, notas/, propuestas/...
type Store struct {
	root string // data/workspaces
}

func NewStore(dataDir string) *Store {
	return &Store{root: filepath.Join(dataDir, "workspaces")}
}

// ContentHash calcula el SHA-256 de un documento; alimenta el sync
// delta y la detección de drift entre filesystem e índice.
func ContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// Read lee un documento desde el filesystem canónico.
func (s *Store) Read(workspaceSlug, relPath string) ([]byte, error) {
	full := s.resolve(workspaceSlug, relPath)
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("leer documento: %w", err)
	}
	return b, nil
}

// Write guarda un documento, creando los directorios intermedios.
func (s *Store) Write(workspaceSlug, relPath string, content []byte) error {
	full := s.resolve(workspaceSlug, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0o644)
}

// Delete elimina un documento del filesystem canónico.
func (s *Store) Delete(workspaceSlug, relPath string) error {
	return os.Remove(s.resolve(workspaceSlug, relPath))
}

// List recorre el árbol de un workspace y devuelve los .md relativos.
func (s *Store) List(workspaceSlug string) ([]string, error) {
	root := filepath.Join(s.root, workspaceSlug)
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

func (s *Store) resolve(workspaceSlug, relPath string) string {
	// saneamiento: nunca escapar de la raíz del workspace
	clean := filepath.Clean(relPath)
	if clean == "." {
		return filepath.Join(s.root, workspaceSlug)
	}
	return filepath.Join(s.root, workspaceSlug, clean)
}
