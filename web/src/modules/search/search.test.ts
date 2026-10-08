// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";

describe("search dedup", () => {
  test("títulos duplicados local+servidor se deduplican", () => {
    const norm = (s: string) => s.trim().toLowerCase();
    const hits = [
      { kind: "task", title: "tarea con prioridad", sub: "@kora" },
      { kind: "task", title: "tarea con prioridad", sub: "tarea" },
      { kind: "task", title: "Tarea con prioridad", sub: "otro" },
    ];
    const seen = new Set<string>();
    const deduped: typeof hits = [];
    for (const h of hits) {
      const key = h.kind + ":" + norm(h.title);
      if (!seen.has(key)) {
        seen.add(key);
        deduped.push(h);
      }
    }
    expect(deduped).toHaveLength(1);
    expect(deduped[0].sub).toBe("@kora"); // el primero gana
  });

  test("diferentes kinds no se deduplican entre sí", () => {
    const norm = (s: string) => s.trim().toLowerCase();
    const hits = [
      { kind: "task", title: "Design System v3" },
      { kind: "doc", title: "Design System v3" },
    ];
    const seen = new Set<string>();
    const deduped: typeof hits = [];
    for (const h of hits) {
      const key = h.kind + ":" + norm(h.title);
      if (!seen.has(key)) {
        seen.add(key);
        deduped.push(h);
      }
    }
    expect(deduped).toHaveLength(2); // task y doc son distintos
  });
});
