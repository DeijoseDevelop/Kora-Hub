// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package web embebe el frontend Nix.js compilado (ADR-04: un solo
// artefacto). El directorio dist/ se genera con `make build-frontend`
// a partir de ../../web y queda ignorado por git.
package web

import "embed"

//go:embed dist/*
var Dist embed.FS

// FS devuelve el sistema de archivos embebido con el frontend.
func FS() embed.FS { return Dist }
