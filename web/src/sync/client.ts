// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { applyChanges, getCursor, type Change } from "./local";
import { queueDB, type QueuedCommand } from "./queue";
import { apiFetch, getToken } from "../api/client";
import { activeWs } from "../data/workspace";
import { showToast } from "../ui/kit";
import { t } from "../i18n";

// Cliente del sync delta (sección 9): push de la cola offline + pull
// del delta por cursor. Replays con idempotency-key se deduplican en
// el servidor.

let inFlight = false;
let dirty = false;

// pushPending envía la cola de comandos al servidor y aplica el delta
// resultante. Si llega un comando durante un push en vuelo (dirty), se
// relanza al terminar en vez de perderse el reintento.
export async function pushPending(): Promise<void> {
  if (inFlight) {
    dirty = true;
    return;
  }
  inFlight = true;
  try {
    do {
      dirty = false;
      await pushOnce();
    } while (dirty);
  } finally {
    inFlight = false;
  }
}

async function pushOnce(): Promise<void> {
  const ws = activeWs.value;
  if (!ws || !getToken()) return;
  await pull(ws); // primero converger: menos conflictos LWW

  // Comandos pendientes (solo lectura): se borran por id SOLO tras un
  // push exitoso — un fallo de red no pierde datos y un comando que se
  // encole durante el push en vuelo sobrevive al bulkDelete.
  const pending = await queueCommands();
  if (pending.length === 0) return;

  try {
    const delta = await apiFetch<{ cursor: number; changes: Change[] }>(
      "/sync/push" + (ws ? "?workspace=" + encodeURIComponent(ws) : ""),
      {
        method: "POST",
        body: JSON.stringify({ commands: pending.map((c) => c.payload) }),
      },
    );
    await applyChanges(delta.changes ?? [], delta.cursor ?? (await getCursor(ws ?? undefined)), ws ?? "");
    await queueDB.commands.bulkDelete(pending.map((c) => c.id!));
    // notificar sync exitoso (solo si hubo comandos)
    window.dispatchEvent(new CustomEvent("hub:sync-done", { detail: { count: pending.length } }));
  } catch (e) {
    // 403 = sin permiso tras refresh válido: se descarta la cola (el
    // servidor decidió) y se avisa. 401/transitorio: la cola se
    // conserva intacta y se reintenta al reconectar (nunca perder
    // datos — ver AGENTS: offline-first).
    const msg = (e as Error).message ?? String(e);
    const code = (e as { code?: string }).code;
    if (code === "forbidden" || msg.includes("HTTP 403")) {
      // solo los comandos de ESTE push: los que se encolaron después
      // (dirty) sobreviven — el comentario de arriba es una promesa
      await queueDB.commands.bulkDelete(pending.map((c) => c.id!));
      showToast(t("sync.rejected"));
    }
  }
}

// pull trae el delta del workspace desde su cursor local y lo aplica.
export async function pull(workspaceId: string): Promise<void> {
  if (!getToken()) return;
  const since = await getCursor(workspaceId);
  const delta = await apiFetch<{ cursor: number; changes: Change[] }>(
    `/sync/changes?workspace=${encodeURIComponent(workspaceId)}&since=${since}`,
  ).catch(() => null);
  if (delta) {
    await applyChanges(delta.changes ?? [], delta.cursor ?? since, workspaceId);
  }
}

async function queueCommands(): Promise<QueuedCommand[]> {
  return queueDB.commands.orderBy("createdAt").toArray();
}

// sha256 del contenido para el mirror (Web Crypto).
export async function sha256(content: string): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(content));
  return Array.from(new Uint8Array(buf)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

// arranque del sync automático: al volver la red se replaya la cola.
export function initSyncAuto(): void {
  window.addEventListener("online", () => {
    void pushPending();
  });
  window.addEventListener("focus", () => {
    const ws = activeWs.value;
    if (!ws) return;
    void pull(ws);
    void pushPending();
  });
}
