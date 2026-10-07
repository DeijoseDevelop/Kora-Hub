# Architecture

This document describes the technical architecture of Kora Hub. It is a public, condensed version of the internal engineering specification.

## Design principles

| # | Principle | Consequence |
|:--|:--|:--|
| P1 | **Files are the truth** | Markdown on disk/S3 is the canonical source; the database is a rebuildable index, never the primary source |
| P2 | **Offline-first from birth** | Every mutation goes through a local command queue with replay; no feature assumes connectivity |
| P3 | **One artifact** | A single Go binary with an embedded frontend and SQLite by default; one container, one volume, zero external dependencies |
| P4 | **API-first** | Everything the UI can do exists in the versioned public REST API; the UI is just another client |
| P5 | **Resource budget** | <100 MB RAM per instance, <10 s cold start; every feature is evaluated against this budget |
| P6 | **One client codebase** | Nix.js compiles to web/PWA and native apps via Capacitor, no extra UI framework |

## High-level view

```
┌───────────────────────────────────────────────┐
│ CLIENT — one codebase                         │
│ Nix.js SPA → PWA (browser) | Capacitor (apps)  │
│ signals · html`` · router · stores · Nix Query │
│ Local persistence: IndexedDB / SQLite-WASM     │
│ Offline queue (CommandQueueAdapter) + delta    │
└──────────────────────────┬────────────────────┘
                           │ HTTPS / REST (JSON)
┌──────────────────────────▼────────────────────┐
│ GO BINARY — one process                       │
│ HTTP (Gin) · JWT · task parser · FTS5           │
│ ├─ SQLite (modernc.org/sqlite) — index & cache │
│ ├─ Markdown store — canonical files (disk/S3)  │
│ └─ Attachments — S3 interface (local/R2/MinIO) │
└───────────────────────────────────────────────┘
```

## Key architectural decisions (summary)

| ADR | Decision | Discarded alternatives |
|:--|:--|:--|
| ADR-01 | Go backend | NestJS/Node (more RAM), Rust (slower dev), Elixir (small ecosystem) |
| ADR-02 | Embedded SQLite by default | Postgres-only (breaks P3 and the one-liner Docker) |
| ADR-03 | sqlc instead of an ORM | GORM/Ent (reflection, magic, RAM) |
| ADR-04 | Nix.js embedded with `embed.FS` | Separate SPA artifact, SSR |
| ADR-05 | Capacitor directly, no Ionic | Nix Ionic (extra layer for a productivity app) |
| ADR-06 | Delta sync + LWW; CRDT deferred | CRDT from day one (6 months of complexity) |
| ADR-07 | Stateless JWT + rotating refresh | Server-side sessions (shared state) |

## Repository layout

```
cmd/hub/          → binary entry point (config, db, migrations, HTTP)
internal/
  auth/           → JWT, bcrypt, middleware, rotating refresh
  config/         → validated environment variables
  docs/           → document CRUD over the canonical filesystem
  tasks/          → embedded task parser (the core asset)
  indexer/        → canonical files → index (tasks/FTS/backlinks)
  sync/           → delta by cursor, push with idempotency keys + LWW
  search/         → FTS5
  graph/          → backlink extraction for the graph view
  server/         → Gin router, REST v1 handlers
  web/            → embedded Nix.js frontend (embed.FS)
db/
  migrations/     → forward-only versioned SQL
  queries/        → sqlc queries
web/              → Nix.js frontend (Vite + Capacitor)
```

## Data model: files + index

The Markdown files are canonical. The SQLite database is an index rebuilt from them:

- `docs` — path, title, `content_hash` (SHA-256) for drift detection and delta sync
- `tasks` — parsed from checkbox lines, anchored by `doc_id + line_no`
- `backlinks` — extracted `[[wiki-links]]`
- `docs_fts` — FTS5 search index
- `change_log` — monotonic cursor per workspace for sync

Deleting the database is a safe operation: `POST /api/v1/admin/reindex` rebuilds everything.

### Embedded task grammar

```
task := checkbox WS text (WS metadata)*
checkbox := '- [ ]' | '- [x]' | '- [~]'      # ~ = in progress
date := '#' (AAAA-MM-DD | 'hoy' | 'mañana' | 'lun'..'dom')
project := '@' ident        priority := '!' (baja|media|alta|1..3)
assignee := '~' ident       tag := '+' ident
```

The parser is idempotent, tolerant and round-trip guaranteed: editing from any view rewrites the original line preserving everything else byte for byte.

## Offline sync

- Every document carries `content_hash` and `updated_at`; the server keeps a monotonic cursor per workspace.
- **Pull**: `GET /sync/changes?since=<cursor>` returns only the delta, with full document snapshots.
- **Push**: clients send commands with an `idempotency_key`; replays are deduplicated server-side.
- **Conflicts**: last-write-wins in v1; the losing version is always preserved in `doc_versions`.

## API (REST v1)

| Group | Endpoints |
|:--|:--|
| Auth | `POST /auth/register` · `POST /auth/login` · `POST /auth/refresh` · `POST /auth/logout` |
| Workspaces | `GET/POST /workspaces` · `POST /workspaces/:id/members` |
| Docs | `GET/POST /docs` · `GET/PATCH/DELETE /docs/:id` · `GET /docs/:id/versions` |
| Tasks | `GET /tasks?vista=kanban\|table\|calendar` · `POST /tasks` (Quick Add) · `PATCH /tasks/:id` |
| Search | `GET /search?q=` |
| Sync | `GET /sync/changes?since=` · `POST /sync/push` |
| System | `GET /healthz` · `GET /version` |

Conventions: `/api/v1` prefix, JSON, Bearer auth, cursor pagination, `{error: {code, message}}` envelope, ULID ids.

## Security

- bcrypt (cost 12) + rotating hashed refresh tokens with per-device revocation
- sqlc generates parameterized queries; dynamic SQL is banned by internal lint
- XSS-hardened by the framework (text-node interpolation) + sanitized Markdown + strict CSP
- Role middleware per handler (owner / editor / viewer); the UI only reflects, the backend guarantees
- E2E-encrypted sync service planned (the server never sees plaintext content)
- Telemetry: opt-in, anonymous, publicly documented; absent by default

## Performance budget (CI-measured)

| Metric | Target |
|:--|:--|
| RAM idle / under 50-user load | <60 MB / <100 MB |
| Cold start (with migrations) | <10 s |
| API latency p95 (local) | <50 ms |
| Frontend TTI (web, 3G) | <3 s |
| Reindex of 10,000 docs | <60 s |
