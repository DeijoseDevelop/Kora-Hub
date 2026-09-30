import "fake-indexeddb/auto";
import assert from "node:assert";
import { test } from "vitest";
import { queueDB, enqueue, queueLength } from "./queue";
import { setSession } from "../api/client";
import { activeWs } from "../data/workspace";
import { pushPending } from "./client";

// Regresión (2026-09-29): la cola se vaciaba ANTES de que el push
// tuviera éxito — un fallo de red perdía las mutaciones offline.

function mockFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  const orig = globalThis.fetch;
  globalThis.fetch = handler as typeof fetch;
  return () => {
    globalThis.fetch = orig;
  };
}

function loginFixture() {
  localStorage.clear();
  setSession({ access_token: "tok", refresh_token: "ref", expires_in: 900 });
  activeWs.value = "ws-test";
}

const PULL_OK = () =>
  new Response(JSON.stringify({ cursor: 0, changes: [] }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

test("push fallido conserva la cola para el próximo replay", async () => {
  await queueDB.commands.clear();
  loginFixture();
  const restore = mockFetch(async (url) => {
    if (String(url).includes("/sync/push")) throw new TypeError("offline");
    return PULL_OK();
  });
  try {
    await enqueue({ key: "doc.upsert", payload: { op: "doc.upsert", path: "a.md" }, idempotencyKey: "k1" });
    await pushPending();
    assert.strictEqual(await queueLength(), 1, "la cola debe sobrevivir al push fallido");
  } finally {
    restore();
  }
});

test("un comando encolado durante un push en vuelo no se borra", async () => {
  await queueDB.commands.clear();
  loginFixture();
  const restore = mockFetch(async (url, init) => {
    if (String(url).includes("/sync/push")) {
      // encola otro comando DURANTE el push: solo deben borrarse los enviados
      await enqueue({ key: "doc.upsert", payload: { op: "doc.upsert", path: "b.md" }, idempotencyKey: "k2" });
      assert.strictEqual(init?.method, "POST");
      return new Response(JSON.stringify({ cursor: 1, changes: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return PULL_OK();
  });
  try {
    await enqueue({ key: "doc.upsert", payload: { op: "doc.upsert", path: "a.md" }, idempotencyKey: "k1" });
    await pushPending();
    assert.strictEqual(await queueLength(), 1, "el comando nuevo quedó en cola");
  } finally {
    restore();
  }
});

test("push exitoso vacía la cola y el comando llega al servidor", async () => {
  await queueDB.commands.clear();
  loginFixture();
  let pushed = "";
  const restore = mockFetch(async (url, init) => {
    if (String(url).includes("/sync/push")) {
      pushed = String(init?.body);
      return new Response(JSON.stringify({ cursor: 1, changes: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return PULL_OK();
  });
  try {
    await enqueue({ key: "doc.upsert", payload: { op: "doc.upsert", path: "a.md" }, idempotencyKey: "k1" });
    await pushPending();
    assert.strictEqual(await queueLength(), 0);
    assert.ok(pushed.includes("a.md"), "el comando viajó en el push");
  } finally {
    restore();
  }
});
