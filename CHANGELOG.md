# Changelog

Todos los cambios notables de Kora Hub se documentan aquí.
Formato basado en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y
[Semantic Versioning](https://semver.org/lang/es/).

## [Unreleased]

## [0.2.0] — 2026-09-29

### Added

- **Gestión de miembros de workspace**: `GET/PATCH/DELETE /workspaces/:id`,
  `GET/POST /workspaces/:id/members` (por email, con rol) y
  `PATCH/DELETE …/members/:uid` con protección del último owner.
  Panel de gestión en Ajustes (visible solo para owners).
- **Adjuntos**: `POST/GET/DELETE /attachments` con whitelist MIME verificada
  por sniffing del contenido, límite `HUB_MAX_UPLOAD_MB`, backend `local`
  (`data/attachments/`) y `s3` (minio-go, envs `HUB_S3_*`). Drag&drop y
  pegar en el editor suben el archivo e insertan el enlace Markdown.
- **Historial de versiones**: cada PATCH guarda el snapshot anterior como
  archivo en `.versions/<doc>/`; `GET /docs/:id/versions` y `/:vid`
  exponen la lista y el contenido. Panel "Historial" en el editor con
  diff visual línea a línea (añadidas/eliminadas).
- **Backlinks por documento**: `GET /docs/:id/backlinks` + panel en el
  editor con los docs que enlazan a este (resolución por título/path).
- **Seguridad (sección 11)**: rate limiting por IP en `/auth/login`,
  `/auth/register` y `/auth/refresh` (5/min), por usuario en `/sync/push`
  (60/min); cabeceras `X-Content-Type-Options`, `X-Frame-Options`,
  `Referrer-Policy`, `Permissions-Policy`, CSP en la app y HSTS solo tras TLS.
- **API**: paginación por cursor en `GET /docs` (`?cursor` + `?limit` →
  `next_cursor`) y filtro `?tipo=doc|tarea|adjunto` en `/search`.
- **Plantillas de documento**: propuesta, acta de reunión, RFC y
  retrospectiva al crear un documento (funciona offline).
- **Export/import de workspace**: `GET /workspaces/:id/export` descarga
  el árbol canónico en ZIP y `POST /workspaces/:id/import` vuelca un
  vault (Obsidian u otro) al workspace — merge idempotente por ruta,
  binarios del ZIP registrados como adjuntos, rutas maliciosas y
  directorios ocultos (`.obsidian/`, `.trash/`) descartados, reindex
  automático. Botones Exportar/Importar en Ajustes.
- **Importador de Notion**: el mismo endpoint detecta automáticamente
  el export de Notion (sufijo de 32 hex en los nombres) y normaliza:
  nombres de archivo/carpeta sin ID, enlaces internos reescritos y
  bases de datos CSV convertidas a documentos con tabla Markdown.
- **Vistas guardadas de tareas** (D1): `GET/POST/DELETE /views` por
  workspace con filtros serializados; selector en la página de Tareas
  con caché local para aplicarlas offline.
- **Gramática de tareas v2** (D2, spec §6.5): `*every:<n>d|w|m|y`
  (al completar se genera la siguiente ocurrencia con la fecha
  recalculada, en el backend y en el mirror local), `^id:<slug>`
  (identidad estable de línea), `^blocked-by:<slug>` (la tarea se
  muestra «bloqueada» mientras la bloqueante no esté hecha) y valores
  de metadatos con `"comillas"`. Parser Go + espejo TS con tests en
  ambos lados; migración `007`.
- **Servidor MCP** (D3): `POST /api/v1/mcp` (JSON-RPC 2.0, transporte
  streamable HTTP, protocolo 2025-06-18) con 9 tools espejo 1:1 de la
  REST — mismo Bearer, mismo resolver de workspace y mismos roles.
- **i18n ES/EN** (D4): capa `t()` con signal de locale, catálogos
  ES/EN en paridad (~120 claves), autodetección por el navegador,
  persistencia en localStorage y selector en Ajustes; las plantillas
  de documentos se generan en el idioma activo.

### Fixed

- `backlinks.dst_doc_id` tenía `REFERENCES docs(id)` pero guarda el texto
  del wikilink: con `foreign_keys(1)` todo insert fallaba y la tabla
  quedó vacía desde el inicio. Migración `005` recrea la tabla sin el FK.
- `docs.Store.resolve` permitía `..` (path traversal); ahora confina las
  rutas al directorio del workspace.
- `Store.List` indexaba directorios ocultos (`.versions/`): excluidos.
- El sync LWW solo guardaba el hash de la versión perdedora (snapshot
  irrecuperable); ahora escribe el contenido en `.versions/`.
- Test con fecha fija `2026-09-01` caducada → fecha siempre futura.

## [0.1.2] — 2026-08-16

### Added

- **Autenticación completa de punta a punta**:
  - Pantalla de registro real (tabs Entrar/Crear cuenta); en el primer
    arranque (sin usuarios) "Crear cuenta" es la opción por defecto.
  - **Refresh rotativo**: `POST /auth/refresh` público rota el par de
    tokens (el viejo queda revocado, el nuevo persistido con hash);
    renovación silenciosa en la UI ante 401 (single-flight).
  - Guardia de sesión: sin token solo se ve la pantalla auth; rutas
    internas protegidas con `meta.auth` + `beforeEach`.
  - `GET /auth/me` (usuario en Ajustes) y `GET /auth/status`
    (`has_users` para el primer arranque).
  - **Onboarding**: registrarse crea el workspace "Personal" automáticamente.
  - Cerrar sesión desde Ajustes revoca el refresh.
  - Fix pérdida de datos: la cola offline ya no se descarta en 401
    (solo en 403 tras refresh válido).

## [0.1.1] — 2026-08-16

### Added

- **App de escritorio (Tauri 2)**: shell nativo que embebe el binario Go como
  sidecar en `127.0.0.1` con datos locales (100% offline). Instalables para
  Linux (`.deb`/`.AppImage`), Windows (`.msi`/`.exe` NSIS) y macOS (`.dmg`
  Intel y Apple Silicon), construidos en CI por plataforma.
- Splash de arranque, single-instance, cierre limpio del sidecar.
- `desktop/` aislado del resto: `Makefile` (`desktop-dev`/`desktop-build`),
  workflow `desktop.yml`, iconos generados.

## [0.1.0] — 2026-08-15

Primera release pública. MVP completo de la Fase 0 y parte de la Fase 1
(licencia AGPL, repositorio público, CI, data layer local-first).

### Added

- **Producto**
  - Workspace self-hosted: documentos Markdown canónicos + índice reconstruible (SQLite/FTS5)
  - Tareas embebidas en los documentos: `- [ ] tarea #fecha @proyecto !prioridad ~asignado +etiqueta`
  - Vistas generadas del índice: kanban (3 columnas con drag & drop), tabla y calendario
  - Quick Add Magic (Ctrl+K): crear tareas en lenguaje natural desde cualquier pantalla
  - Búsqueda full-text FTS5 con tolerancia tipográfica
  - Grafo de conocimiento en canvas con backlinks `[[wikilinks]]` reales
  - Editor Markdown con preview en vivo (CodeMirror 6 + marked)
  - Workspaces con roles: owner / editor / viewer
  - Command palette universal (Ctrl+K) con búsqueda difusa
  - PWA instalable
- **Offline-first (data layer local)**
  - Parser de tareas portado a TypeScript (misma gramática que el backend)
  - Índice local en el dispositivo (Dexie v3): docs, tareas y backlinks
  - La UI lee del mirror local; edición y creación 100% sin conexión
  - Cola de comandos con idempotency-key y replay al reconectar
  - Sync por delta (cursor monótono por workspace) con LWW y versión perdedora conservada
- **API REST v1**
  - Auth JWT stateless con refresh rotativo (bcrypt)
  - CRUD de docs sobre el filesystem canónico, tareas (vistas por parámetro), búsqueda, grafo
  - Sync: `GET /sync/changes?since=` y `POST /sync/push`
- **Distribución**
  - Licencia **AGPL-3.0-only** con headers SPDX y NOTICE
  - Imagen Docker multi-stage (`scratch`, <40 MB)
  - GoReleaser: binarios para linux/darwin/windows (amd64/arm64)
  - GitHub Actions: CI (test, typecheck, vitest, sqlc diff) + release

### Fixed

- Serialización de tareas: los campos nullable se exponen como `string|null`
  (antes `sql.NullString`, que rompía la UI)
- LWW con reloj del cliente: una cola offline multi-comando ya no pierde el
  último cambio al reconectar
- Reindexado incremental por `content_hash` (sin drift)
- Bugs del framework Nix.js documentados y sorteado: interpolaciones de
  atributos con espacios, imports lazy intermitentes → carga directa

### Changed

- Rediseño completo de la UI (sistema de diseño v2): tema claro, un acento,
  Inter + JetBrains Mono self-hosted
- Bundle dividido en chunks (main ~22 KB gzip)

## [0.0.1] — 2026-08-14

Versión de desarrollo (no publicada).

- Esqueleto del repositorio: `cmd/hub`, `internal/*`, `db/`, `web/`
- Stack instalado a sus últimas versiones (Go 1.26, Gin 1.12, sqlc 1.31,
  Nix.js 2.6, Capacitor 8)
