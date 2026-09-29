# AGENTS.md — Kora Hub

Reglas obligatorias para cualquier agente o persona que trabaje en este repositorio.

## Fuente de verdad: el vault de Obsidian

El producto está definido en el vault `~/Documents/Obsidian/Kora Hub`. **Antes de empezar cualquier tarea se leen las notas del vault que la afectan.** El código implementa lo que el vault define; si algo no está definido, se define primero en el vault.

| Si la tarea toca… | Leer primero |
| --- | --- |
| Cualquier cosa | `Home.md` · `00 - Inicio/MOC - Kora Hub.md` · `02 - Desarrollo/Estado del Desarrollo.md` |
| Producto, scope, prioridades, negocio | `01 - Documentos Maestros/01 - Documento Maestro de Producto y Negocio v1.0.md` (MoSCoW §9.2, roadmap §14, anti-posicionamiento §7.3) |
| Arquitectura, ADRs, stack, API, datos, sync | `01 - Documentos Maestros/02 - Documento Tecnico de Arquitectura v1.0.md` (principios P1-P6 §1, ADRs §2.2, API §4.2, modelo de datos §5, parser §6, sync §9) |
| Versiones reales instaladas | `02 - Desarrollo/Stack y Versiones Instaladas.md` (desviaciones del doc técnico: Capacitor 8.5, Go 1.26.6, `@deijose/nix-js-testing`) |
| Bugs conocidos del stack | `02 - Desarrollo/Estado del Desarrollo.md` (quirks de Nix.js, sqlc, Dexie — sección QA) |

- IDs citados en código y comentarios: principios `P1`–`P6`, `ADR-01`–`ADR-07`, secciones del doc técnico (`§4.2`, `§6`). No se inventan IDs.
- Si el código y el vault se contradicen **no se elige uno en silencio**: se corrige lo que esté mal y se deja constancia en `Estado del Desarrollo.md`.
- Toda decisión nueva (ADR) o cambio de contrato se refleja en el vault **en el mismo cambio**.

## Principios de producto no negociables

Están congelados en el vault (P1-P6 + §9.3). Resumen operativo:

1. **Los archivos son la verdad**: Markdown canónico en `data/`; SQLite es índice reconstruible. Si una feature exige una entidad separada de los documentos, se rediseña o se rechaza.
2. **Offline-first**: si no funciona sin conexión, no se construye. Toda mutación pasa por la cola local con replay.
3. **Un solo artefacto**: binario Go con frontend embebido. Si necesita más de un contenedor/volumen, ya falló.
4. **API-first**: toda acción de la UI existe en la API REST. Sin excepciones.
5. **Presupuesto**: <100 MB RAM por instancia, arranque <10 s, bundle frontend <200 KB gzip.
6. **Un solo codebase de cliente**: Nix.js sirve web/PWA, Capacitor y el shell Tauri de escritorio.

## Idiomas

| Contexto | Idioma |
| --- | --- |
| Código: variables, funciones, tipos, paquetes | **Inglés** |
| Nombres de archivos y rutas | **Inglés** (snake_case en SQL) |
| Campos JSON y columnas de base de datos | **Inglés** snake_case |
| Comentarios de código y doc comments | **Español** |
| Vault, CHANGELOG, docs internas | **Español** |
| README y `docs/` (público, GTM internacional) | **Inglés** — decisión registrada en `Estado del Desarrollo.md` |
| Textos de la interfaz | Español (producto nacido en ES; la API devuelve códigos de error, no textos) |
| Commits y PRs | Conventional commits en español o inglés (el historial usa ambos). **Commits profesionales: mensaje claro del porqué, sin marcas de agua ni trailers de herramientas (nada de "Generated with", "Co-Authored-By" de bots, ni menciones a agentes de IA)** |

## Licencia y headers

- **AGPL-3.0-only**. Todo archivo fuente nuevo (Go, TS, CSS, SQL, Dockerfiles, Makefile) lleva el header SPDX:

  ```text
  // Copyright (C) 2026 Deijose <tech@deijose.dev>
  // SPDX-License-Identifier: AGPL-3.0-only
  ```

- El open-core es estratégico (vault §8): el producto self-hosted es gratuito y completo; nunca se bloquea una feature para forzar pago (nada de crippleware).

## Testing (obligatorio)

Nada se considera terminado sin su prueba. El parser de tareas es el componente cuyo fallo corrompe datos de usuario: cobertura de ramas prioritaria.

| Capa | Comando | Qué cubre |
| --- | --- | --- |
| Backend | `make test` (`go test ./...`) | Parser (golden cases), indexer, sync (LWW, idempotencia), auth (rotación/revocación), roles, quickadd |
| Frontend | `cd web && npm test` (vitest) | Parser TS espejo del Go, palette, páginas |
| Tipos | `make vet` (`go vet` + `tsc --noEmit`) | Ambos lados |
| Build completo | `make build` | Frontend → embed → binario único |
| E2E | Playwright sobre el binario real | Escenario crítico: crear doc con tareas offline → cerrar → reconectar → kanban |

- Tests nombrados por comportamiento: `TestPatchTask_RoundTripPreservesMetadata`.
- El parser existe dos veces (Go `internal/tasks` y TS `web/src/tasks`): **toda gramática nueva se implementa y se prueba en ambos** — los tests TS son espejo de los Go.
- Datos de prueba siempre ficticios. Toda corrección de bug empieza con un test que falla.

## Go (backend)

- Stack fijado: **Gin + sqlc + modernc.org/sqlite** (driver puro Go, `CGO_ENABLED=0`), `golang-jwt/v5`, `oklog/ulid`. Stdlib primero; sin ORM (ADR-03).
- `ctx` siempre como primer parámetro; errores internos envueltos con `%w`; prohibido `panic` fuera de `main`.
- Errores de API con el sobre `{"error": {"code": "...", "message": "..."}}`: `code` estable snake_case en inglés (el cliente lo traduce), `message` técnico.
- sqlc: los `.sql` de `db/queries/` van **en ASCII puro** — bug confirmado de sqlc v1.31: comentarios con acentos corrompen las constantes generadas. Tras tocar queries, `make generate` y commitear el código generado (CI verifica el diff).
- Migraciones en `db/migrations/` (`NNN_nombre.sql`), **forward-only**, embebidas en el binario; backup automático pre-migración.
- La DB es índice reconstruible (P1): `POST /api/v1/admin/reindex` debe poder reconstruir todo desde los archivos. Ningún dato canónico solo en SQLite.
- Comentarios GoDoc en español empezando por el nombre del símbolo (`// Router construye…`); comentarios inline solo para el **porqué**.

## Frontend (Nix.js)

Seguir las skills `nix-js`, `nix-query` y `nix-js-auth`. Prohibido JSX: templates `html`, `signal`/`computed`/`effect`, `createStore`, `createQuery`/`createCommand` (modo `queueOffline` para mutaciones).

Trampas verificadas en QA (todas reales, no repetir):

- **Interpolación de atributos completa o nada**: `class=${"tab " + x}` funciona, `class="tab ${x}"` rompe el parser. Atención extra cuando el prefijo estático termina en espacio.
- **Imports eager en el router**: `lazy()` producía estados vacíos intermitentes; el bundle es ~22 KB gzip, la navegación instantánea gana.
- **Refetch en `onMount`**: queries module-level cachean antes del login y quedan stale; cada página reconsulta al montar.
- **Null-checks con `== null`**: las queries devuelven `undefined` en vuelo, no `null`.
- **`data-testid` no existe**: el parser descarta atributos con `-`; usar `id` para tests y E2E.
- **vitest**: un `import` de `expect` extra rompe el render de páginas (quirk de evaluación de módulos); tests por archivo con `node:assert`.
- **Escribir signals desde callbacks de CodeMirror** (updateListener) provoca re-mount en loop: leer con `getDoc()`, no sincronizar por keystroke.

Data layer local-first (`web/src/data`, `web/src/sync`, `web/src/platform`):

- **La UI lee del mirror local** (Dexie), nunca directo del servidor. El servidor solo sincroniza.
- Toda mutación → cola con `idempotency-key` → `POST /sync/push` al reconectar; rollback con snapshot en fallo definitivo.
- En 401 la cola **no** se descarta (solo en 403 tras refresh válido): sin pérdida de datos.
- `PlatformStorage` abstrae web (IndexedDB/SQLite-WASM) vs nativo (`@capacitor-community/sqlite`): ningún módulo de negocio conoce la diferencia.

## Desktop (Tauri 2)

`desktop/` es un shell Rust (~3 MB) que lanza el **mismo binario** como sidecar en `127.0.0.1` — no es un cliente aparte. Reglas: `single-instance` (protege `hub.db`), `HUB_JWT_SECRET` generado y persistido 0600, cierre limpio del sidecar. El Go/frontend no se toca desde `desktop/` salvo la capa de plataforma.

## Datos y seguridad

- Migraciones SQL versionadas; tablas nuevas con `UNIQUE`/constraints que reflejen la invariante real.
- bcrypt costo 12; refresh tokens rotativos hasheado-en-DB; revocación atómica.
- Autorización por workspace (owner/editor/viewer) enforced por middleware en el backend — la UI solo refleja permisos, nunca los garantiza.
- Conflictos de sync: LWW por documento en v1, **la versión perdedora siempre se preserva** en `doc_versions` (nunca destructivo).
- Telemetría: ausente por defecto; si llega, opt-in, anónima y documentada públicamente.
- Secretos solo por variables de entorno (`HUB_*`).

## Estructura

```text
cmd/hub/         entry point: config, db, migraciones, HTTP
internal/
  auth/          JWT, bcrypt, middleware, refresh rotativo
  config/        env vars validadas (HUB_*)
  db/            código sqlc generado + open/migraciones
  docs/          store canónico de Markdown (filesystem = verdad)
  tasks/         parser de tareas embebidas (el activo central)
  indexer/       archivos → índice (tareas, FTS5, backlinks), incremental por content_hash
  sync/          delta por cursor, push con idempotency + LWW
  search/        FTS5
  graph/         extracción de backlinks para el grafo
  server/        router Gin + handlers REST v1
  web/           frontend compilado embebido (embed.FS)
  attachments/   interfaz Storage: local (data/attachments) + S3 (minio-go)
  webhooks/      (pendiente) eventos doc.saved / task.*
db/migrations/   SQL forward-only embebido
db/queries/      queries sqlc (ASCII puro — ver sección Go)
web/             frontend Nix.js (Vite + Capacitor)
  src/api        cliente HTTP + role
  src/data       mirror local-first (Dexie) — la UI lee de aquí
  src/sync       cola offline, push/pull, idempotency
  src/tasks      parser TS (espejo del Go)
  src/modules    docs, tasks, search, graph, settings, home
  src/app        shell, command palette
desktop/         app de escritorio Tauri 2 (sidecar del binario)
docs/            docs públicas en inglés (ARCHITECTURE.md)
scripts/         tooling de desarrollo
screenshots/     capturas reales para README (se regeneran con la app corriendo)
```

Directorios vacíos en `internal/` son placeholders de módulos planeados en el doc técnico §4.1 — no eliminarlos sin actualizar el vault.

## Estado y roadmap

El avance se trackea en `02 - Desarrollo/Estado del Desarrollo.md` (fases 0-4 con regla de avance: ninguna fase empieza sin el hito anterior). Fase 0 (MVP dogfood) ✅ completa; fase 1 (lanzamiento OSS) en curso.

Al completar trabajo relevante: actualizar `Estado del Desarrollo.md` y `CHANGELOG.md` (Keep a Changelog, español) en el mismo cambio.

## Definition of Done

1. `make test`, `make vet` verdes; `make build` produce el binario; CI en verde (sqlc diff-check incluido).
2. Tests nuevos incluidos y verdes; el bug reproduce con test que falla antes del fix.
3. Headers SPDX en archivos nuevos; sin código muerto, sin TODO sin referencia al vault.
4. Cambios de contrato/gramática: parser Go + parser TS + ambos tests en el mismo cambio.
5. Vault actualizado si cambia una decisión, un contrato o el estado del roadmap.
6. README/`docs/` en inglés y CHANGELOG en español, actualizados según corresponda.
