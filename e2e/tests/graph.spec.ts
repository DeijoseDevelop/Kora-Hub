import { test, expect, request, type APIRequestContext } from "@playwright/test";

const PASSWORD = "e2e-password-123";
let session: { access_token: string; refresh_token: string };
let api: APIRequestContext;

test.beforeAll(async ({ request: req }) => {
  api = await request.newContext({ baseURL: "http://127.0.0.1:8099" });
  for (let i = 0; i < 4; i++) {
    const res = await api.post("/api/v1/auth/register", {
      data: { email: `graph-${Date.now()}-${Math.floor(Math.random()*1e6)}@kora.test`, password: PASSWORD, display_name: "Graph" },
    });
    if (res.ok()) { session = await res.json(); return; }
    if (res.status() === 429) { await new Promise(r => setTimeout(r, 12000)); continue; }
    throw new Error(`register: ${res.status()}`);
  }
});

test("grafo renderiza nodos y backlinks", async ({ page }) => {
  await page.addInitScript(([t, r]) => {
    localStorage.setItem("hub:token", t);
    localStorage.setItem("hub:refresh", r);
    localStorage.setItem("hub:expires", String(Date.now() + 3600000));
  }, [session.access_token, session.refresh_token]);

  // crear dos docs con wikilink entre ellos
  const h = { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" };
  await api.post("/api/v1/docs", { headers: h, data: { path: "doc-a.md", title: "Doc A", content: "# Doc A\n\nVer [[Doc B]] para más.\n" } });
  await api.post("/api/v1/docs", { headers: h, data: { path: "doc-b.md", title: "Doc B", content: "# Doc B\n\nVolver a [[Doc A]].\n" } });

  await page.goto("/#/graph");
  // el canvas existe y tiene dimensiones reales
  const canvas = page.locator(".graph-canvas");
  await expect(canvas).toBeVisible();
  const box = await canvas.boundingBox();
  expect(box).toBeTruthy();
  expect(box!.width).toBeGreaterThan(100);
  expect(box!.height).toBeGreaterThan(100);

  // el canvas tiene contenido pintado (no es un rectángulo vacío)
  const hasInk = await page.evaluate(() => {
    const c = document.querySelector(".graph-canvas") as HTMLCanvasElement;
    if (!c) return false;
    const ctx = c.getContext("2d");
    if (!ctx) return false;
    const d = ctx.getImageData(0, 0, c.width, c.height).data;
    for (let i = 3; i < d.length; i += 4) if (d[i] > 0) return true;
    return false;
  });
  expect(hasInk).toBe(true);
});
