// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from "vitest/config";
import nix from "@deijose/vite-plugin-nix-js";

export default defineConfig({
  plugins: [nix()],
  base: "/",
  build: {
    target: "es2022",
    outDir: "dist",
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes("codemirror") || id.includes("@codemirror")) return "codemirror";
          if (id.includes("marked") || id.includes("flexsearch") || id.includes("dexie")) return "vendor";
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      // P4: la UI es un cliente más de la API REST v1
      "/api": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
});
