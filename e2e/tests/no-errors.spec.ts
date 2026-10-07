// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, request, type APIRequestContext } from "@playwright/test";

// Regresión: ni effect loops (Elur glitch-free) ni CSP fonts (data:).
const PASSWORD = "e2e-password-123";
let session: { access_token: string; refresh_token: string };
let api: APIRequestContext;

test.beforeAll(async ({ request: req }) => {
  api = await request.newContext({ baseURL: "http://127.0.0.1:8099" });
  for (let i = 0; i < 4; i++) {
    const res = await api.post("/api/v1/auth/register", {
      data: { email: `noerr-${Date.now()}-${Math.floor(Math.random()*1e6)}@kora.test`, password: PASSWORD, display_name: "T" },
    });
    if (res.ok()) { session = await res.json(); return; }
    if (res.status() === 429) { await new Promise(r => setTimeout(r, 12000)); continue; }
    throw new Error(`register: ${res.status()}`);
  }
});

test("sin errores de consola en toda la navegación", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (msg) => { if (msg.type() === "error") errors.push(msg.text()); });
  page.on("pageerror", (err) => errors.push("PAGEERROR: " + err.message));
  await page.addInitScript(([t, r]) => {
    localStorage.setItem("hub:token", t);
    localStorage.setItem("hub:refresh", r);
    localStorage.setItem("hub:expires", String(Date.now() + 3600000));
  }, [session.access_token, session.refresh_token]);
  for (const route of ["/#/docs", "/#/tasks", "/#/graph", "/#/search", "/#/settings"]) {
    await page.goto(route);
    await page.waitForTimeout(800);
  }
  const relevant = errors.filter((e) => !e.includes("favicon") && !e.includes("net::"));
  expect(relevant, `errores de consola: ${relevant.join(" | ")}`).toHaveLength(0);
});
