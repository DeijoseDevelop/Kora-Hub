// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import Dexie, { type Table } from "dexie";

// Cola offline (P2): toda mutación pasa por una cola local con replay;
// sobrevive al cierre del navegador (respaldada por IndexedDB).

export interface QueuedCommand {
  id?: number;
  key: string; // p.ej. "tasks/save"
  payload: unknown;
  idempotencyKey: string;
  createdAt: number;
  attempts: number;
}

class HubQueueDB extends Dexie {
  commands!: Table<QueuedCommand, number>;

  constructor() {
    super("kora-hub");
    this.version(1).stores({
      commands: "++id, idempotencyKey, createdAt",
    });
  }
}

export const queueDB = new HubQueueDB();

export async function enqueue(cmd: Omit<QueuedCommand, "id" | "createdAt" | "attempts">): Promise<number> {
  return queueDB.commands.add({
    ...cmd,
    createdAt: Date.now(),
    attempts: 0,
  });
}

export async function dequeue(): Promise<QueuedCommand | undefined> {
  const first = await queueDB.commands.orderBy("createdAt").first();
  if (!first?.id) return undefined;
  await queueDB.commands.delete(first.id);
  return first;
}

export async function clearQueue(): Promise<void> {
  await queueDB.commands.clear();
}

export async function queueLength(): Promise<number> {
  return queueDB.commands.count();
}
