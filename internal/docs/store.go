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
// Los directorios ocultos (.versions, .attachments) no son documentos:
// quedan excluidos del índice.
func (s *Store) List(workspaceSlug string) ([]string, error) {
	root := filepath.Join(s.root, workspaceSlug)
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
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

// WriteVersion guarda un snapshot fuera del árbol de documentos
// (.versions/<docID>/<ulid>.md) y devuelve su ruta relativa.
func (s *Store) WriteVersion(workspaceSlug, docID, name string, content []byte) (string, error) {
	rel := filepath.Join(".versions", docID, name+".md")
	if err := s.Write(workspaceSlug, rel, content); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// ReadVersion lee un snapshot de .versions por su ruta relativa.
func (s *Store) ReadVersion(workspaceSlug, relPath string) ([]byte, error) {
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if !strings.HasPrefix(clean, ".versions/") {
		return nil, fmt.Errorf("ruta fuera de .versions: %s", relPath)
	}
	return s.Read(workspaceSlug, clean)
}

func (s *Store) resolve(workspaceSlug, relPath string) string {
	// saneamiento: el slug y el path nunca escapan de data/workspaces
	root := filepath.Join(s.root, safeSeg(workspaceSlug))
	clean := filepath.Clean(relPath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return root
	}
	if strings.ContainsRune(clean, 0) {
		return root
	}
	full := filepath.Join(root, clean)
	if rel, err := filepath.Rel(root, full); err != nil || strings.HasPrefix(rel, "..") {
		return root
	}
	return full
}

// safeSeg neutraliza un segmento de ruta que se supone simple (slug de
// workspace): sin separadores ni puntos suspensivos.
func safeSeg(seg string) string {
	if seg == "" || seg == "." || seg == ".." {
		return "_"
	}
	if strings.ContainsAny(seg, `/\`) || strings.ContainsRune(seg, 0) {
		return "_"
	}
	return seg
}

// ValidSlug reporta si un slug de workspace es seguro como directorio
// (letras minúsculas, dígitos y guiones; sin puntos ni barras).
func ValidSlug(slug string) bool {
	if len(slug) < 1 || len(slug) > 63 {
		return false
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	// ni '..' ni guiones extremos/dobles que confundan al filesystem
	if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") || strings.Contains(slug, "--") {
		return false
	}
	return true
}
