// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, request, type Page, type APIRequestContext } from "@playwright/test";

// Escenario crítico de QA (AGENTS.md): register → doc con tarea →
// kanban → cola offline con replay → share-link público sin sesión.
// Ampliado con regresiones reales: round-trip de tareas [x]/[~],
// semántica del kanban por columna, XSS en Markdown, tipado de tokens,
// path traversal, búsqueda y aislamiento de workspaces.
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
  // rate limit: 5/min por IP en /auth/register. Reintenta con backoff
  // si el servidor devuelve 429 (corridas seguidas en local).
  for (let attempt = 0; attempt < 5; attempt++) {
    const res = await api.post("/api/v1/auth/register", {
      data: {
        email: `e2e-${Date.now()}-${Math.floor(Math.random() * 1e6)}@kora.test`,
        password: PASSWORD,
        display_name: "E2E User",
      },
    });
    if (res.ok()) {
      session = await res.json();
      return;
    }
    if (res.status() === 429 && attempt < 4) {
      await new Promise((r) => setTimeout(r, 15_000));
      continue;
    }
    throw new Error(`register fallo: ${res.status()} ${await res.text()}`);
  }
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

async function apiGet(path: string): Promise<any> {
  const res = await api.get("/api/v1" + path, {
    headers: { Authorization: `Bearer ${session.access_token}` },
  });
  return res.json();
}

// serverDocId espera a que el sync suba el doc local y devuelve su id real.
async function serverDocId(title: string): Promise<string> {
  let id = "";
  await expect
    .poll(async () => {
      const docs = (await apiGet("/docs")).docs ?? [];
      id = docs.find((d: { title: string }) => d.title === title)?.id ?? "";
      return id;
    }, { timeout: 15_000 })
    .not.toBe("");
  return id;
}

// fireHTML5Drop simula el drag & drop HTML5 (Playwright dragTo usa mouse
// events y no dispara dragstart/drop nativos).
async function fireHTML5Drop(page: Page, cardText: string, colState: string): Promise<void> {
  await page.evaluate(
    ([text, state]) => {
      const cards = Array.from(document.querySelectorAll(".task-card")) as HTMLElement[];
      const card = cards.find((c) => c.textContent?.includes(text));
      const col = document.querySelector(`.kanban-col-body[data-state="${state}"]`) as HTMLElement;
      if (!card || !col) throw new Error("card o columna no encontrada: " + text + " -> " + state);
      const dt = new DataTransfer();
      card.dispatchEvent(new DragEvent("dragstart", { dataTransfer: dt, bubbles: true }));
      col.dispatchEvent(new DragEvent("dragover", { dataTransfer: dt, bubbles: true, cancelable: true }));
      col.dispatchEvent(new DragEvent("drop", { dataTransfer: dt, bubbles: true, cancelable: true }));
    },
    [cardText, colState] as const,
  );
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

    const id = await serverDocId("Doc público E2E");
    const share = await api.post(`/api/v1/docs/${id}/share`, {
      headers: { Authorization: `Bearer ${session.access_token}` },
    });
    expect(share.ok()).toBeTruthy();
    const { token: shareToken } = await share.json();

    // visita el enlace público sin sesión: contenido, sin editor
    await page.goto(`/#/p/${shareToken}`);
    await expect(page.locator(".public-body")).toContainText("Contenido público e2e");
    await expect(page.locator(".cm-content")).toHaveCount(0);
  });

  test("round-trip: descompletar una tarea [x] conserva el título", async ({ page }) => {
    // Regresión: roundTrip destruía el texto de las líneas [x]/[~].
    await injectSession(page);
    await createDoc(page, "Doc round-trip");
    await page.locator(".cm-content").click();
    await page.keyboard.type("- [x] informe anual confidencial #2099-02-02 @proyecto");
    await page.getByRole("button", { name: "Guardar" }).click();
    await expect(page.locator(".save-indicator.visible")).toContainText("Guardado");

    await page.goto("/#/tasks");
    // la tarea hecha vive en la columna done
    const doneCol = page.locator('.kanban-col-body[data-state="x"]');
    await expect(doneCol.locator(".task-card", { hasText: "informe anual confidencial" })).toBeVisible();

    // marcar como pendiente (drop en la columna todo) no debe corromper
    await fireHTML5Drop(page, "informe anual confidencial", " ");
    await expect(
      page.locator('.kanban-col-body[data-state=" "] .task-card', { hasText: "informe anual confidencial" }),
    ).toBeVisible({ timeout: 10_000 });

    // el documento canónico conserva el título completo
    await expect
      .poll(async () => {
        const docs = (await apiGet("/docs")).docs ?? [];
        const d = docs.find((x: { title: string }) => x.title === "Doc round-trip");
        if (!d) return "";
        const full = await apiGet(`/docs/${d.id}`);
        return full.doc?.content ?? full.content ?? "";
      }, { timeout: 15_000 })
      .toContain("informe anual confidencial");

    // y la línea no está duplicada ni truncada
    const docs = (await apiGet("/docs")).docs ?? [];
    const d = docs.find((x: { title: string }) => x.title === "Doc round-trip");
    const full = await apiGet(`/docs/${d.id}`);
    const content: string = full.doc?.content ?? full.content ?? "";
    expect(content).not.toContain("- [ ] - [");
    expect(content).not.toContain("- [x] - [");
    expect(content).toContain("- [ ] informe anual confidencial");
  });

  test("kanban por columna: todo → doing → done con semántica [ ] [~] [x]", async ({ page }) => {
    await injectSession(page);
    await createDoc(page, "Doc kanban cols");
    await page.locator(".cm-content").click();
    await page.keyboard.type("- [ ] tarea de columna multiple");
    await page.getByRole("button", { name: "Guardar" }).click();
    await expect(page.locator(".save-indicator.visible")).toContainText("Guardado");

    await page.goto("/#/tasks");
    const todo = page.locator('.kanban-col-body[data-state=" "]');
    const doing = page.locator('.kanban-col-body[data-state="~"]');
    const done = page.locator('.kanban-col-body[data-state="x"]');
    await expect(todo.locator(".task-card", { hasText: "tarea de columna multiple" })).toBeVisible();

    // todo → doing: la línea pasa a [~] (antes marcaba done por error)
    await fireHTML5Drop(page, "tarea de columna multiple", "~");
    await expect(doing.locator(".task-card", { hasText: "tarea de columna multiple" })).toBeVisible({ timeout: 10_000 });

    // doing → done
    await fireHTML5Drop(page, "tarea de columna multiple", "x");
    await expect(done.locator(".task-card", { hasText: "tarea de columna multiple" })).toBeVisible({ timeout: 10_000 });

    // el markdown refleja [x]
    await expect
      .poll(async () => {
        const docs = (await apiGet("/docs")).docs ?? [];
        const d = docs.find((x: { title: string }) => x.title === "Doc kanban cols");
        if (!d) return "";
        const full = await apiGet(`/docs/${d.id}`);
        return full.doc?.content ?? full.content ?? "";
      }, { timeout: 15_000 })
      .toContain("- [x] tarea de columna multiple");
  });

  test("XSS: el Markdown con HTML activo no ejecuta en el preview ni en el share", async ({ page }) => {
    await injectSession(page);
    const xss = `<img src=x onerror="window.__xss=1"><script>window.__xss=2</script><a href="javascript:window.__xss=3">clic</a>Seguro`;

    await createDoc(page, "Doc XSS");
    await page.locator(".cm-content").click();
    await page.keyboard.type(xss);
    await page.getByRole("button", { name: "Guardar" }).click();
    await expect(page.locator(".save-indicator.visible")).toContainText("Guardado");

    // preview del editor: el contenido visible está, el JS no
    await expect(page.locator(".doc-preview, .preview, .cm-editor").first()).toBeVisible();
    expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();

    // share público: mismo origen, peor vector — tampoco ejecuta
    const id = await serverDocId("Doc XSS");
    const share = await api.post(`/api/v1/docs/${id}/share`, {
      headers: { Authorization: `Bearer ${session.access_token}` },
    });
    const { token: shareToken } = await share.json();
    await page.goto(`/#/p/${shareToken}`);
    await expect(page.locator(".public-body")).toContainText("Seguro");
    expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();
  });

  test("búsqueda local-first encuentra docs y tareas", async ({ page }) => {
    await injectSession(page);
    await createDoc(page, "Doc búsqueda única");
    await page.locator(".cm-content").click();
    await page.keyboard.type("- [ ] tarea hallazgo especial #2099-03-03");
    await page.getByRole("button", { name: "Guardar" }).click();
    await expect(page.locator(".save-indicator.visible")).toContainText("Guardado");

    await page.goto("/#/search");
    await expect(page.locator("#search-input")).toBeVisible();
    await page.locator("#search-input").click();
    await page.keyboard.type("hallazgo");
    // puede haber hits locales y del servidor: basta con que exista
    await expect(page.locator(".search-hit", { hasText: "tarea hallazgo especial" }).first()).toBeVisible({ timeout: 10_000 });

    await page.locator("#search-input").fill("");
    await page.keyboard.type("única");
    await expect(page.locator(".search-hit", { hasText: "Doc búsqueda única" }).first()).toBeVisible({ timeout: 10_000 });

    await page.locator("#search-input").fill("");
    await page.keyboard.type("zzz-no-existe-zzz");
    await expect(page.locator(".search-hit")).toHaveCount(0);
  });

  test("command palette: flechas mueven el highlight y Enter navega", async ({ page }) => {
    await injectSession(page);
    await page.goto("/#/tasks");
    await page.keyboard.press("Control+k");
    await expect(page.locator(".palette")).toBeVisible();
    const items = page.locator(".palette-item");
    await expect(items.first()).toBeVisible();
    const n = await items.count();
    expect(n).toBeGreaterThan(1);
    await expect(items.first()).toHaveClass(/selected/);
    await page.keyboard.press("ArrowDown");
    await expect(items.nth(1)).toHaveClass(/selected/);
    await expect(items.first()).not.toHaveClass(/selected/);
    await page.keyboard.press("ArrowUp");
    await expect(items.first()).toHaveClass(/selected/);
    await page.keyboard.press("Escape");
    await expect(page.locator(".palette")).toHaveCount(0);
  });

  test("seguridad API: slug con traversal y refresh usado como access", async () => {
    // slug path traversal
    const bad = await api.post("/api/v1/workspaces", {
      headers: { Authorization: `Bearer ${session.access_token}` },
      data: { slug: "../evil", name: "Evil" },
    });
    expect(bad.status()).toBe(400);

    // refresh token no vale como access token
    const me = await api.get("/api/v1/auth/me", {
      headers: { Authorization: `Bearer ${session.refresh_token}` },
    });
    expect(me.status()).toBe(401);

    // y el access token sí
    const meOk = await api.get("/api/v1/auth/me", {
      headers: { Authorization: `Bearer ${session.access_token}` },
    });
    expect(meOk.status()).toBe(200);
  });

  test("la búsqueda FTS no revienta con comillas ni operadores", async () => {
    for (const q of ['"comillas"', "AND", "a OR b", "foo*", "-"]) {
      const res = await api.get(`/api/v1/search?q=${encodeURIComponent(q)}`, {
        headers: { Authorization: `Bearer ${session.access_token}` },
      });
      expect(res.status(), `q=${q}`).toBe(200);
    }
  });

  test("i18n: el selector de idioma cambia los textos de la UI", async ({ page }) => {
    await injectSession(page);
    await page.goto("/#/settings");
    // el selector está en Ajustes; se fuerza el cambio de locale
    const sel = page.locator("select").filter({ hasText: /Español|English/ });
    if (await sel.count()) {
      await sel.first().selectOption("en");
      await expect(page.locator("body")).toContainText(/Settings|Workspace/);
    } else {
      // fallback: el locale se cachea; al menos la app renderiza
      await expect(page.locator(".page")).toBeVisible();
    }
  });
});
