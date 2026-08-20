// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package graph extrae los backlinks entre documentos (sintaxis
// [[dobles corchetes]]) que alimentan el grafo de relaciones.
package graph

import (
	"regexp"
	"strings"
)

var wikiLink = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)

// ExtractBacklinks devuelve los destinos enlazados desde un contenido.
// anchor_text es el texto visible del enlace, si existe.
type Link struct {
	Dest   string `json:"dest"`
	Anchor string `json:"anchor"`
}

func ExtractBacklinks(content string) []Link {
	var out []Link
	for _, m := range wikiLink.FindAllStringSubmatch(content, -1) {
		dest := strings.TrimSpace(m[1])
		if dest == "" {
			continue
		}
		anchor := dest
		if i := strings.Index(dest, "#"); i >= 0 {
			anchor = dest[i+1:]
		}
		out = append(out, Link{Dest: dest, Anchor: anchor})
	}
	return out
}
