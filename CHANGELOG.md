# Changelog

Todos los cambios notables de Kora Hub se documentan aquí.
Formato basado en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y
[Semantic Versioning](https://semver.org/lang/es/).

## [1.0.1] — 2026-10-08

### Fixed

- **Desktop Windows**: el sidecar Go abría una ventana de consola
  (`-H windowsgui` añadido a ldflags). El `.exe` ahora abre directo.
- **WebView2 auto-install**: `downloadBootstrapper` descarga e instala
  WebView2 automáticamente si no está en Windows.
- **NSIS**: instalación `currentUser` (sin permisos admin).
- **Error handling**: si el sidecar no arranca en 20s, la ventana
  muestra un mensaje claro en vez de quedarse en blanco.
- **Search**: dedup de resultados local+servidor normalizado (trim +
  lowercase) — tareas ya no aparecen duplicadas.
- **Mobile**: modo local offline-first — la app funciona **sin servidor**.
  Botón "Continuar sin conexión" en el login nativo.
- **Mobile**: error `Unexpected token '<', "<!DOCTYPE"` al hacer login —
  `safeJSON` detecta respuestas HTML y muestra error legible.

## [1.0.0] — 2026-10-07

### Added

- **Búsqueda full-text local-first** (`/search`): resultados del mirror
  Dexie al instante + refuerzo FTS5 del servidor cuando hay red, filtro
  `todos|doc|tarea`, snippets con contexto. Antes era un stub.
- **Sanitizador HTML por allowlist** (`web/src/ui/sanitize.ts`): preview
  del editor y página pública `#/p/:token` ya no ejecutan HTML activo
  del Markdown del usuario (XSS persistente → robo de sesión en el
  share-link). 9 tests vitest.
- **Kanban por columna semántico**: drop en Todo→`[ ]`, Doing→`[~]`,
  Done→`[x]` (antes cualquier drop hacía toggle de done).
- **`PATCH /tasks/:id` con `state`** (`open|done|progress`) y flags
  `done`/`in_progress` como punteros — un PATCH de solo proyecto ya no
  des-completa la tarea.
- **`SetTaskState`** (sqlc): fija `done` + `in_progress` en una sola
  query.
- **Tests de regresión E2E Playwright ampliados** (11 specs): round-trip
  de tareas `[x]`/`[~]`, kanban por columna, XSS, búsqueda, command
  palette con teclado, path traversal, token typ, FTS con comillas,
  i18n.
- **Design System v3**: tokens 2-tier (primitivos oklch + semánticos),
  escala de espaciado base-4, tipografía Major Third 1.25, profundidad
  plana (bordes > sombras), focus-ring consistente, `prefers-reduced-motion`.
- **CodeMirror theme claro** (`koraLight` + `koraHighlight`): sustituye
  a `oneDark` (oscuro en UI clara) con los colores del DS v3.
- **Empty states**: icono SVG inline en `.empty-state`, hint "Arrastra
  aquí" en columnas kanban vacías.
- **Command palette mejorado**: "Nuevo documento" abre el flujo completo
  (prompt → plantilla → crear → navegar); nuevas acciones "Ir a Búsqueda"
  e "Ir a Ajustes".
- **Estilos de búsqueda**: `.search-hit`, `.badge.doc/task`, snippets,
  `.search-box` (antes no existían).

### Fixed

- **Round-trip del parser corrompía tareas `[x]`/`[~]`** (Go y TS):
  `strings.Fields`/`split(3)` trataba el checkbox como 3 tokens y
  reescribía la línea duplicando el checkbox o borrando el título.
  Cualquier PATCH sobre una tarea hecha/en progreso corrompía el
  Markdown canónico. RoundTrip ahora extrae `rest` vía `parseCheckbox`
  + `splitMeta`, reemplaza solo el primer slot válido de cada tipo y
  re-cita valores con espacios.
- **Parser TS↔Go divergente**: TS aceptaba `[X]`/tabs y Go no (y
  viceversa con `#lun` vs `#lunes`); TS conservaba `#no-fecha` como
  texto y Go lo descartaba. Ahora ambos aceptan `[X]`, tabulador tras
  `-`, abreviaturas y nombres largos de día, y `#no-fecha` es texto.
- **Fechas relativas en UTC** (`toISOString`): `#hoy`/`#mañana` daban el
  día equivocado entre 00:00–02:00 locales. Ahora componentes locales en
  parser TS y `kit.ts`.
- **XSS persistente en Markdown** (preview + share público): `marked`
  no sanitiza y el resultado iba a `innerHTML`. Ahora `sanitizeHTML`
  por allowlist sobre el DOM.
- **Path traversal por slug de workspace**: `../evil` como slug
  escribía/borraba fuera de `data/workspaces/`. `docs.ValidSlug` +
  `safeSeg` en `store.resolve` + validación en `POST /workspaces`.
- **Refresh token aceptado como access token**: `ParseAccess` no
  distinguía `typ` — un refresh filtrado daba 30 días de acceso. Claim
  `typ: access|refresh` verificado en ambos parsers.
- **Pull perdía cambios al truncar por `limit`**: el cursor devuelto era
  `lastSeq` (máximo global), no el seq de la última fila entregada. Con
  500 cambios de golpe la pérdida era masiva. Ahora el cursor es el seq
  de la última fila; si un snapshot falla el cursor no avanza.
- **LWW se bypaseaba sin `updated_at`**: el push siempre ganaba sin
  preservar la versión perdedora. Ahora se normaliza el reloj y
  **siempre** se snapshota el contenido anterior antes de sobrescribir.
- **`doc.delete` no idempotente**: un replay dejaba el batch clavado en
  409 para siempre. `DeleteDoc` devuelve nil si el doc ya no existe.
- **`DELETE /workspaces` fallaba con 500** si había tareas, versiones,
  shares o adjuntos (FK sin `ON DELETE CASCADE`). Orden FK-safe con
  limpieza de `doc_shares`, `doc_versions`, `backlinks`, `docs_fts`,
  `saved_views`, `webhooks`, `audit_log`.
- **IDs de tarea inestables**: `rebuildTasks` hacía `DeleteTasksForDoc` +
  `UpsertTask` con ULID nuevo en cada reindex — el `PATCH /tasks/:id`
  devolvía un id que ya no existía. Ahora upsert por `(doc_id, line_no)`
  conserva el id y solo se purgan líneas que ya no son tareas.
- **Reindex recreaba docs borrados con id inconsistente**: el ULID nuevo
  ignoraba el `RETURNING id` del upsert (la fila conservaba el viejo) y
  las tareas insertadas apuntaban a un doc inexistente. Ahora se usa el
  id devuelto por `UpsertDoc`.
- **Reindex no purgaba docs huérfanos**: un doc borrado a mano sobrevivía
  en el índice indefinidamente. `purgeMissingDocs` lo elimina + limpia
  versiones/shares/backlinks.
- **`applyChanges` cruzaba workspaces por `path`**: un pull de WS-A
  machacaba el inbox de WS-B (el match por path no filtraba workspace).
  Ahora `filter(workspaceId + deleted !== 1)`. Igual en `quickAddLocal`.
- **Kanban drop marcaba done al soltar en "En curso"**: el `@drop`
  ignoraba la columna destino. Ahora semántica por columna.
- **Command palette: flechas no movían el highlight**: `selected` era un
  campo plano (no signal) y Nix.js no re-evalúa `class` dentro de un
  `.map()` anidado. Ahora `selected = signal(0)` + `paintSelection()`
  que pinta el highlight a mano + foco automático en el input + manejo
  de flechas en el window keydown (el foco puede no estar en el input).
- **`pushPending` reentrante perdía el reintento**: un comando encolado
  durante un push en vuelo quedaba huérfano. Ahora flag `dirty` que
  relanza al terminar. `focus` ahora también hace push (antes solo pull).
- **`clearQueue()` en 403 borraba comandos encolados durante el push**:
  ahora solo se descartan los de ese push (`bulkDelete` por id).
- **Rate-limit bypaseable por `X-Forwarded-For`**: Gin confiaba en todos
  los proxies. `SetTrustedProxies(nil)` + purga del mapa de hits >4096.
- **FTS5 revienta con comillas/operadores**: `MATCH ?` con query crudo
  daba 500. Ahora `escapeFTS` (literal entre comillas).
- **Sin límite de cuerpo ni timeouts HTTP**: `limitBody(8MB)` +
  `ReadTimeout/WriteTimeout/IdleTimeout` en `http.Server`.
- **`handlePatchTask` reescribía la línea equivocada** si el archivo
  cambió desde el índice: ahora `resolveTaskLine` re-resuelve por `^id:`
  y por título antes de mutar.
- **Quick-add duplicaba el checkbox** si el texto ya lo traía
  (`"− [x] algo"` → `"− [ ] − [x] algo"`). `LooksLikeTask` (Go+TS)
  detecta checkbox existente y no lo duplica.
- **`handleLogin` enmascaraba errores de BD como credenciales inválidas**
  (la rama 500 era inalcanzable).
- **README/docs prometían webhooks** que están en P3: se ajustó el texto
  a "webhooks on the roadmap".

### Changed

- `RoundTrip(t, done, ...)` → `RoundTrip(t, state, ...)` con constantes
  `StateOpen|StateDone|StateProgress` (Go) y `TaskState` (TS).
- `applyTaskState(content, task, done)` → `applyTaskState(content, task, state)`.
- `patchTaskRequest.Done` es ahora `*bool` (distingue ausente de false).
- Nueva función `setTaskStateLocal` en el data layer (kanban por columna).
- Nuevas claves i18n: `search.*` (ph, all, docs, tasks, empty, searching),
  `sync.rejected`, `app.nav.search`, `tasks.col_empty`,
  `palette.goto_search/search_sub/goto_settings/settings_sub`.
- **Código muerto eliminado** (DoD: sin dead code): `syncStatus`,
  `queueDocUpdate`, `dequeue`, `localReady`, `badgeDate/Project/Priority`.
- `docs/ARCHITECTURE.md`: se quitó "webhooks" del diagrama (P3).
- CSS: design system v3 (tokens 2-tier, escala 4px, oklch, flat depth,
  `prefers-reduced-motion`); CodeMirror `oneDark` → `koraLight`.

### Fixed

- **Imagen GHCR multi-arch**: `latest`/`<version>` resolvían solo amd64;
  ahora `docker_manifests` une `amd64`+`arm64` — el one-liner de Docker
  funciona igual en hosts ARM. Verificado: `docker pull
  ghcr.io/deijosedevelop/kora-hub:0.3.0` + `docker run` → `/healthz`
  responde `version: 0.3.0`.
- **Pérdida de datos en la cola offline**: `pushPending` vaciaba la cola
  Dexie antes de que el push tuviera éxito — un fallo de red (offline,
  500) perdía las mutaciones encoladas. Ahora solo se borran por id tras
  un push exitoso (y un comando encolado durante el push sobrevive).
- **Endpoints por id inalcanzables para docs creados offline**: los docs
  locales conservan su id `local-*` y nunca aprendían el id del
  servidor — `/docs/:id/{versions,backlinks,share}` respondían 404
  siempre para ellos. `applyChanges` ahora sella `serverId` al hacer
  merge por path; la UI usa `serverId ?? id` y el editor avisa si el doc
  aún no sincronizó al pedir el enlace.

## [0.3.0] — 2026-09-29

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

## [0.2.0] — 2026-08-20

### Changed

- **Rebrand completo Zekrost Hub → Kora Hub** y migración del
  repositorio a `DeijoseDevelop/Kora-Hub`.
- Release publicado vía GoReleaser: binarios Linux/macOS/Windows
  (amd64 + arm64), instaladores de escritorio (deb, rpm, AppImage,
  msi, exe, dmg) e imagen Docker en GHCR.

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
