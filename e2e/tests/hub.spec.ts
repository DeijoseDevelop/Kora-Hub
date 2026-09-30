// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, request, type Page, type APIRequestContext } from "@playwright/test";

// Escenario crítico de QA (AGENTS.md): register → doc con tarea →
// kanban → cola offline con replay → share-link público sin sesión.
//
// Rate limiting: /auth/* está limitado a 5 req/min por IP. Un solo
// register API por worker (beforeAll) + la sesión se inyecta en
// localStorage vía addInitScript — los tests de UI no gastan cuota.

const PASSWORD = "e2e-password-123";
let session: { access_token: string; refresh_token: string; expires_in?: number };
let baseURL: string;
let api: APIRequestContext;

test.beforeAll(async ({ request: req }) => {
  baseURL = test.info().project.use.baseURL as string;
  api = await request.newContext({ baseURL });
  const res = await api.post("/api/v1/auth/register", {
    data: {
      email: `e2e-${Date.now()}-${Math.floor(Math.random() * 1e6)}@kora.test`,
      password: PASSWORD,
      display_name: "E2E User",
    },
  });
  expect(res.ok()).toBeTruthy();
  session = await res.json();
});

// injectSession deja la sesión activa antes de que cargue el bundle.
async function injectSession(page: Page): Promise<void> {
  await page.addInitScript(
    ([t, r, e]) => {
      localStorage.setItem("hub:token", t);
      localStorage.setItem("hub:refresh", r);
      localStorage.setItem("hub:expires", e);
    },
    [session.access_token, session.refresh_token, String(Date.now() + (session.expires_in ?? 3600) * 1000)],
  );
}

async function createDoc(page: Page, title: string): Promise<void> {
  await page.goto("/#/docs");
  await page.locator("#new-doc").click();
  await page.locator(".modal input").fill(title);
  await page.locator('.modal [data-act="confirm"]').click();
  await page.locator(".select-option").first().click(); // plantilla en blanco
  await expect(page.locator(".cm-content")).toBeVisible();
}

test.describe("kora hub e2e", () => {
  test("register → doc con tarea → aparece en el kanban", async ({ page }) => {
    // register por la UI real (el único test que toca el formulario)
    await page.goto("/#/");
    await page.getByRole("button", { name: "Crear cuenta" }).click();
    await page.locator('input[type="text"]').fill("E2E UI User");
    await page.locator('input[type="email"]').fill(`e2e-ui-${Date.now()}@kora.test`);
    await page.locator('input[type="password"]').fill(PASSWORD);
    await page.locator('form button.btn[type="submit"]').click();
    await expect(page.locator("#new-doc")).toBeVisible({ timeout: 10_000 });

    await createDoc(page, "Doc E2E");
    await page.locator(".cm-content").click();
    await page.keyboard.type("- [ ] tarea e2e kanban #2099-01-01");
    await page.getByRole("button", { name: "Guardar" }).click();
    await expect(page.locator(".save-indicator.visible")).toContainText("Guardado");

    await page.goto("/#/tasks");
    await expect(page.locator(".task-card", { hasText: "tarea e2e kanban" })).toBeVisible();
  });

  test("offline: quick-add encola y llega al servidor al reconectar", async ({ page, context }) => {
    await injectSession(page);
    await page.goto("/#/tasks");
    await expect(page.locator(".quick-add-btn")).toBeVisible();

    // sin red: la tarea aparece igual (offline-first, mirror local)
    await context.setOffline(true);
    await page.locator('input[type="text"]').fill("tarea offline e2e");
    await page.locator(".quick-add-btn").click();
    await expect(page.locator(".task-text", { hasText: "tarea offline e2e" })).toBeVisible();

    // aún no llegó al servidor (el contexto API no está offline)
    const before = await api.get("/api/v1/tasks?done=0", {
      headers: { Authorization: `Bearer ${session.access_token}` },
    });
    const beforeTasks = (await before.json()).tasks ?? [];
    expect(beforeTasks.some((t: { title: string }) => t.title.includes("offline e2e"))).toBeFalsy();

    // reconectar dispara 'online' → pushPending replaya la cola
    await context.setOffline(false);
    await expect
      .poll(
        async () => {
          const res = await api.get("/api/v1/tasks?done=0", {
            headers: { Authorization: `Bearer ${session.access_token}` },
          });
          const tasks = (await res.json()).tasks ?? [];
          return tasks.some((t: { title: string }) => t.title.includes("offline e2e"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();
  });

  test("share-link público sirve el doc sin sesión (D5)", async ({ page }) => {
    await injectSession(page);
    await createDoc(page, "Doc público E2E");
    await page.locator(".cm-content").click();
    await page.keyboard.type("# Contenido público e2e");
    await page.getByRole("button", { name: "Guardar" }).click();
    await expect(page.locator(".save-indicator.visible")).toContainText("Guardado");

    // El doc local lleva id "local-*"; el servidor indexa por path y le
    // da su propio ULID — lo resolvemos listando docs hasta que llegue
    // el sync (la cola push es inmediata con red, esto cubre el desfase).
    let serverDocId = "";
    await expect
      .poll(async () => {
        const res = await api.get("/api/v1/docs", {
          headers: { Authorization: `Bearer ${session.access_token}` },
        });
        const docs = (await res.json()).docs ?? [];
        serverDocId = docs.find((d: { path: string }) => d.path.endsWith(".md") && d.title === "Doc público E2E")?.id ?? "";
        return serverDocId;
      }, { timeout: 15_000 })
      .not.toBe("");

    const share = await api.post(`/api/v1/docs/${serverDocId}/share`, {
      headers: { Authorization: `Bearer ${session.access_token}` },
    });
    expect(share.ok()).toBeTruthy();
    const { token: shareToken } = await share.json();

    // visita el enlace público sin sesión: contenido, sin editor
    await page.goto(`/#/p/${shareToken}`);
    await expect(page.locator(".public-body")).toContainText("Contenido público e2e");
    await expect(page.locator(".cm-content")).toHaveCount(0);
  });
});
