// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from "@playwright/test";

// E2E sobre el binario real compilado — CI arranca el servidor antes
// (mismo patrón que el load test k6). Un solo worker: SQLite + estado
// compartido no admiten paralelismo entre tests.
export default defineConfig({
  testDir: "./tests",
  workers: 1,
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  use: {
    baseURL: process.env.BASE_URL ?? "http://127.0.0.1:8099",
    locale: "es-ES",
  },
  projects: [
    {
      name: "chromium",
      use: {
        browserName: "chromium",
        // Chrome del sistema: no siempre están los browsers de Playwright
        // cacheados en local; en CI se usa el binario de Playwright.
        ...(process.env.CHROME_BIN ? { launchOptions: { executablePath: process.env.CHROME_BIN } } : {}),
      },
    },
  ],
});
