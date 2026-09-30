import Dexie, { type Table } from "dexie";
import { parse } from "../tasks/parser";

// Mirror local + índice (P1 portado al cliente): los archivos Markdown
// (o su copia local) son la fuente; tasks/backlinks son índices
// reconstruibles desde el contenido.

export interface LocalDoc {
  id: string;
  path: string;
  title: string;
  content: string;
  contentHash: string;
  updatedAt: string;
  deleted: number;
  workspaceId: string;
  // id del servidor, adoptado en el primer sync (los docs locales usan
  // id "local-*" para navegar; los endpoints REST por id necesitan este)
  serverId?: string;
}

export interface LocalTask {
  id: string; // docId::lineNo
  docId: string;
  docTitle: string;
  workspaceId: string;
  lineNo: number;
  title: string;
  dueDate: string | null;
  project: string | null;
  priority: string | null;
  assignee: string | null;
  done: boolean;
  inProgress: boolean;
  recur: string | null;     // *every: (§6.5)
  taskUid: string | null;   // ^id:
  blockedBy: string | null; // ^blocked-by:
}

export interface LocalBacklink {
  srcDocId: string;
  dstTitle: string;
  anchor: string;
}

interface Meta {
  key: string;
  value: string;
}

class HubLocalDB extends Dexie {
  docs!: Table<LocalDoc, string>;
  tasks!: Table<LocalTask, string>;
  backlinks!: Table<LocalBacklink, string>;
  meta!: Table<Meta, string>;

  constructor() {
    super("kora-hub-local");
    this.version(1).stores({
      docs: "id, path, updatedAt",
      meta: "key",
    });
    this.version(2).stores({
      docs: "id, path, updatedAt",
      tasks: "id, docId, project, dueDate, done",
      backlinks: "srcDocId, dstTitle",
      meta: "key",
    });
    this.version(3).stores({
      docs: "id, path, updatedAt, deleted",
      tasks: "id, docId, project, dueDate, done",
      backlinks: "srcDocId, dstTitle",
      meta: "key",
    });
    this.version(4).stores({
      docs: "id, path, workspaceId, updatedAt, deleted",
      tasks: "id, docId, workspaceId, project, dueDate, done",
      backlinks: "srcDocId, dstTitle",
      meta: "key",
    });
    // v5: taskUid indexado para resolver dependencias ^blocked-by:
    this.version(5).stores({
      docs: "id, path, workspaceId, updatedAt, deleted",
      tasks: "id, docId, workspaceId, project, dueDate, done, taskUid",
      backlinks: "srcDocId, dstTitle",
      meta: "key",
    });
  }
}

export const localDB = new HubLocalDB();

export async function getCursor(workspaceId?: string): Promise<number> {
  const key = workspaceId ? "cursor:" + workspaceId : "cursor";
  const row = await localDB.meta.get(key);
  return row ? Number(row.value) : 0;
}

export async function setCursor(cursor: number, workspaceId?: string): Promise<void> {
  const key = workspaceId ? "cursor:" + workspaceId : "cursor";
  await localDB.meta.put({ key, value: String(cursor) });
}

export interface Change {
  seq: number;
  op: string;
  doc?: { id: string; path: string; title: string; content: string; content_hash: string; updated_at: string };
}

// applyChanges aplica un delta del servidor al mirror local.
export async function applyChanges(changes: Change[], cursor: number, workspaceId: string): Promise<void> {
  for (const ch of changes) {
    if (ch.op === "delete") {
      const id = ch.doc?.id;
      if (id) {
        // el delta referencia el id del servidor: el doc local puede
        // tener id "local-*" — se resuelve por serverId si hace falta.
        const existing = (await localDB.docs.get(id))
          ?? (await localDB.docs.filter((d) => d.serverId === id).first());
        if (existing) {
          await localDB.docs.put({ ...existing, deleted: 1 });
          await localDB.tasks.where("docId").equals(existing.id).delete();
        }
      }
      continue;
    }
    if (!ch.doc) continue;
    // Un doc creado localmente (id local-*) conserva su id: la UI ya
    // navega con él. El id del servidor se adopta en `serverId` para los
    // endpoints REST por id (evita duplicados por path sin romper la
    // navegación).
    const byPath = await localDB.docs.where("path").equals(ch.doc.path).first();
    if (byPath) {
      await localDB.docs.update(byPath.id, {
        title: ch.doc.title,
        content: ch.doc.content,
        contentHash: ch.doc.content_hash,
        updatedAt: ch.doc.updated_at,
        deleted: 0,
        workspaceId,
        serverId: ch.doc.id,
      });
      continue;
    }
    await localDB.docs.put({
      id: ch.doc.id,
      path: ch.doc.path,
      title: ch.doc.title,
      content: ch.doc.content,
      contentHash: ch.doc.content_hash,
      updatedAt: ch.doc.updated_at,
      deleted: 0,
      workspaceId,
    });
  }
  await setCursor(cursor, workspaceId);
}

export async function mirrorDoc(doc: {
  id: string;
  path: string;
  title: string;
  content: string;
  contentHash: string;
  updatedAt: string;
  workspaceId: string;
}): Promise<void> {
  await localDB.docs.put({ ...doc, deleted: 0 });
}

// ------------------------- Índice local -------------------------

export async function reindexDoc(docId: string): Promise<number> {
  const doc = await localDB.docs.get(docId);
  if (!doc || doc.deleted) return 0;
  const idxKey = "idx:" + docId;
  const indexed = await localDB.meta.get(idxKey);
  if (indexed?.value === doc.contentHash) return 0; // sin drift

  const tasks = parse(doc.content);
  await localDB.tasks.where("docId").equals(docId).delete();
  await localDB.backlinks.where("srcDocId").equals(docId).delete();

  for (const t of tasks) {
    await localDB.tasks.put({
      id: `${docId}::${t.line}`,
      docId,
      docTitle: doc.title,
      workspaceId: doc.workspaceId,
      lineNo: t.line,
      title: t.title,
      dueDate: t.dueDate,
      project: t.project,
      priority: t.priority,
      assignee: t.assignee,
      done: t.done,
      inProgress: t.inProgress,
      recur: t.recur,
      taskUid: t.taskUid,
      blockedBy: t.blockedBy,
    });
  }
  for (const link of extractBacklinks(doc.content)) {
    await localDB.backlinks.put({ srcDocId: docId, ...link });
  }
  await localDB.meta.put({ key: idxKey, value: doc.contentHash });
  return tasks.length;
}

export async function reindexAll(): Promise<number> {
  const docs = await localDB.docs.where("deleted").equals(0).toArray();
  let n = 0;
  for (const d of docs) n += await reindexDoc(d.id);
  return n;
}

export async function getLocalDocs(workspaceId?: string): Promise<LocalDoc[]> {
  if (!workspaceId) return localDB.docs.where("deleted").equals(0).toArray();
  return localDB.docs.where("workspaceId").equals(workspaceId).filter((d) => d.deleted === 0).toArray();
}

export async function getLocalTasks(workspaceId?: string): Promise<LocalTask[]> {
  if (!workspaceId) return localDB.tasks.toArray();
  return localDB.tasks.where("workspaceId").equals(workspaceId).toArray();
}

export async function getLocalBacklinks(): Promise<LocalBacklink[]> {
  return localDB.backlinks.toArray();
}

// Extrae [[wikilinks]] (port de internal/graph del backend).
const WIKI = /\[\[([^\]|]+)(?:\|[^\]]*)?\]\]/g;
export function extractBacklinks(content: string): Array<{ dstTitle: string; anchor: string }> {
  const out: Array<{ dstTitle: string; anchor: string }> = [];
  for (const m of content.matchAll(WIKI)) {
    const dest = m[1].trim();
    if (!dest) continue;
    const anchor = dest.includes("#") ? dest.slice(dest.indexOf("#") + 1) : dest;
    out.push({ dstTitle: dest, anchor });
  }
  return out;
}
