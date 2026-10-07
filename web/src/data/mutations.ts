import { localDB, mirrorDoc, reindexDoc, type LocalDoc, type LocalTask } from "../sync/local";
import { enqueue } from "../sync/queue";
import { pushPending } from "../sync/client";
import { refreshLocal } from "./store";
import { activeWs } from "./workspace";
import { applyTaskState, nextOccurrence, parse, spawnRecurring } from "../tasks/parser";

export function nowUTC(): string {
  return new Date().toISOString().replace("T", " ").slice(0, 19);
}

export async function sha256(content: string): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(content));
  return Array.from(new Uint8Array(buf)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

// saveDocLocal: escribe el mirror, reindexa, refresca la UI y encola el
// comando de sync (idempotency-key). Es el camino único de mutación.
export async function saveDocLocal(doc: { id: string; path: string; title: string; content: string }): Promise<void> {
  const updatedAt = nowUTC();
  const contentHash = await sha256(doc.content);
  await mirrorDoc({ ...doc, contentHash, updatedAt, workspaceId: activeWs.value ?? "" });
  await reindexDoc(doc.id);
  await refreshLocal();
  const key = crypto.randomUUID();
  await enqueue({
    key: "doc.upsert",
    payload: {
      idempotency_key: key,
      op: "doc.upsert",
      doc_id: doc.id,
      path: doc.path,
      content: doc.content,
      updated_at: updatedAt,
    },
    idempotencyKey: key,
  });
  window.dispatchEvent(new CustomEvent("hub:docs-changed"));
  void pushPending().catch(() => undefined);
}

// createDocLocal: crea el documento en el mirror (funciona offline) y lo
// encola para sincronizar. Devuelve el doc local (su id es la navegación).
// El contenido inicial puede venir de una plantilla (ver data/templates).
export async function createDocLocal(title: string, path: string, initial?: string): Promise<LocalDoc> {
  const id = "local-" + crypto.randomUUID();
  const content = initial ?? `# ${title}\n\n- [ ] Primera tarea #hoy\n\nEmpieza a escribir. Las tareas con \`- [ ]\` aparecerán en el kanban.\n`;
  const doc: LocalDoc = {
    id,
    path,
    title,
    content,
    contentHash: await sha256(content),
    updatedAt: nowUTC(),
    deleted: 0,
    workspaceId: activeWs.value ?? "",
  };
  await saveDocLocal({ id, path, title, content });
  return doc;
}

export async function getLocalDocById(id: string): Promise<LocalDoc | undefined> {
  return localDB.docs.get(id);
}

// setTaskTitleLocal actualiza el título de una tarea reescribiendo la
// línea en el Markdown canónico (round-trip con el texto nuevo).
export async function setTaskTitleLocal(task: LocalTask, newTitle: string): Promise<void> {
  const doc = await localDB.docs.get(task.docId);
  if (!doc) return;
  const tasks = parse(doc.content);
  const t = tasks.find((p) => p.line === task.lineNo);
  if (!t) return;
  const lines = doc.content.split("\n");
  // reconstruir la línea preservando metadatos, cambiando solo el texto
  const meta = [t.dueDate ? `#${t.dueDate}` : "", t.project ? `@${t.project}` : "",
    t.priority ? `!${t.priority}` : "", t.assignee ? `~${t.assignee}` : "",
    ...t.tags.map((tag) => `+${tag}`),
    t.recur ? `*every:${t.recur}` : "",
    t.taskUid ? `^id:${t.taskUid}` : "",
    t.blockedBy ? `^blocked-by:${t.blockedBy}` : "",
  ].filter(Boolean).join(" ");
  const state = t.done ? "x" : t.inProgress ? "~" : " ";
  lines[t.line - 1] = `- [${state}] ${newTitle}${meta ? " " + meta : ""}`;
  await saveDocLocal({ id: doc.id, path: doc.path, title: doc.title, content: lines.join("\n") });
}

// toggleTaskLocal: round-trip local (la línea se reescribe al byte) y
// todo el doc se sincroniza.
export async function toggleTaskLocal(task: LocalTask): Promise<void> {
  const next = task.done ? " " : "x";
  await setTaskStateLocal(task, next);
}

// setTaskStateLocal fija el estado del checkbox (" " | "x" | "~") con
// round-trip local: la línea se reescribe al byte y el doc se sincroniza.
// Es el camino del kanban por columna (todo/doing/done).
export async function setTaskStateLocal(task: LocalTask, state: " " | "x" | "~"): Promise<void> {
  const doc = await localDB.docs.get(task.docId);
  if (!doc) return;
  const tasks = parse(doc.content);
  const t = tasks.find((p) => p.line === task.lineNo);
  if (!t) return;
  let content = applyTaskState(doc.content, t, state);
  // recurrencia (§6.5): al completar, la siguiente ocurrencia se
  // inserta como línea nueva debajo — espejo del backend
  if (state === "x" && !t.done && t.recur && t.dueDate) {
    const next = nextOccurrence(t.dueDate, t.recur);
    if (next) {
      const lines = content.split("\n");
      lines.splice(t.line, 0, spawnRecurring(t.rawLine, next));
      content = lines.join("\n");
    }
  }
  await saveDocLocal({ id: doc.id, path: doc.path, title: doc.title, content });
}

// quickAddLocal: anexa la tarea a Inbox.md local (round-trip del texto
// natural) y sincroniza el doc. Devuelve la tarea creada.
export async function quickAddLocal(text: string): Promise<LocalTask | null> {
  const { parseLine, roundTrip, looksLikeTask } = await import("../tasks/parser");
  const trimmed = text.trim();
  const line = looksLikeTask(trimmed) ? trimmed : "- [ ] " + trimmed;
  const parsed = parseLine(line);
  if (!parsed) return null;
  const canonical = roundTrip(parsed, " ", parsed.dueDate, parsed.project, parsed.priority, parsed.assignee);

  const wsId = activeWs.value ?? "";
  let inbox = await localDB.docs
    .filter((d) => d.path === "inbox.md" && d.workspaceId === wsId && d.deleted !== 1)
    .first();
  let id = inbox?.id;
  let content = inbox?.content ?? "";
  if (!inbox) {
    id = "local-" + crypto.randomUUID();
    content = "# Inbox\n\n";
  }
  const newContent = content.endsWith("\n") ? content + canonical + "\n" : content + "\n" + canonical + "\n";
  await saveDocLocal({ id: id!, path: "inbox.md", title: "Inbox", content: newContent });

  const { parse } = await import("../tasks/parser");
  const created = parse(newContent).pop();
  if (!created) return null;
  return {
    id: `${id}::${created.line}`,
    docId: id!,
    docTitle: "Inbox",
    workspaceId: activeWs.value ?? "",
    lineNo: created.line,
    title: created.title,
    dueDate: created.dueDate,
    project: created.project,
    priority: created.priority,
    assignee: created.assignee,
    done: created.done,
    inProgress: created.inProgress,
    recur: created.recur,
    taskUid: created.taskUid,
    blockedBy: created.blockedBy,
    depth: created.depth,
    parentLine: created.parentLine,
  };
}
