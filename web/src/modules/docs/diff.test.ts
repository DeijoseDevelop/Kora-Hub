// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { lineDiff } from "./diff";

describe("lineDiff", () => {
  it("identicos: todo same", () => {
    const d = lineDiff("a\nb\nc", "a\nb\nc");
    expect(d.every((l) => l.op === "same")).toBe(true);
    expect(d.map((l) => l.text)).toEqual(["a", "b", "c"]);
  });

  it("linea añadida", () => {
    const d = lineDiff("a\nc", "a\nb\nc");
    expect(d).toEqual([
      { op: "same", text: "a" },
      { op: "add", text: "b" },
      { op: "same", text: "c" },
    ]);
  });

  it("linea eliminada", () => {
    const d = lineDiff("a\nb\nc", "a\nc");
    expect(d).toEqual([
      { op: "same", text: "a" },
      { op: "del", text: "b" },
      { op: "same", text: "c" },
    ]);
  });

  it("edicion = del + add", () => {
    const d = lineDiff("hola", "adios");
    expect(d).toEqual([
      { op: "del", text: "hola" },
      { op: "add", text: "adios" },
    ]);
  });

  it("vacio -> contenido", () => {
    const d = lineDiff("", "a\nb");
    // "" produce una "linea" vacia en split("\n")
    expect(d.filter((l) => l.op === "add").length).toBe(2);
  });
});