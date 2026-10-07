import { test, expect } from "@playwright/test";
test("sin errores de consola (effect loop + CSP fonts)", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") errors.push(msg.text());
  });
  page.on("pageerror", (err) => errors.push("PAGEERROR: " + err.message));
  const api = await (await import("@playwright/test")).request.newContext({ baseURL: "http://127.0.0.1:8099" });
  const res = await api.post("/api/v1/auth/register", {
    data: { email: `noerr-${Date.now()}@kora.test`, password: "e2e-password-123", display_name: "T" },
  });
  const s = await res.json();
  await page.addInitScript(([t, r]) => {
    localStorage.setItem("hub:token", t);
    localStorage.setItem("hub:refresh", r);
    localStorage.setItem("hub:expires", String(Date.now() + 3600000));
  }, [s.access_token, s.refresh_token]);
  await page.goto("/#/docs");
  await page.waitForTimeout(1500);
  await page.goto("/#/tasks");
  await page.waitForTimeout(1000);
  await page.goto("/#/graph");
  await page.waitForTimeout(1000);
  await page.goto("/#/search");
  await page.waitForTimeout(1000);
  const relevant = errors.filter((e) => !e.includes("favicon") && !e.includes("net::"));
  console.log("CONSOLE ERRORS:", JSON.stringify(relevant, null, 2));
  expect(relevant).toHaveLength(0);
});
