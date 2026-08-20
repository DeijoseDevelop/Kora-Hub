// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package db contiene el esquema SQL versionado y las queries sqlc.
// El esquema se embebe en el binario único (P3) y se aplica como
// migraciones forward-only (sección 14).
package db

import "embed"

//go:embed migrations/*.sql
var MigrationsFS embed.FS
