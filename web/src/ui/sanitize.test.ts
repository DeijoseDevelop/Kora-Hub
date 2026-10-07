// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test, beforeEach } from "vitest";
import { sanitizeHTML } from "./sanitize";

describe("sanitizeHTML", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
  });

  test("conserva el HTML de Markdown inofensivo", () => {
    const html = "<h1 id=\"t\">Título</h1><p>Hola <strong>mundo</strong></p><ul><li>a</li></ul>";
    const out = sanitizeHTML(html);
    expect(out).toContain("<h1");
    expect(out).toContain("<strong>mundo</strong>");
    expect(out).toContain("<li>a</li>");
  });

  test("elimina script y su contenido activo", () => {
    const out = sanitizeHTML(`<p>ok</p><script>alert(1)</script>`);
    expect(out).not.toContain("script");
    expect(out).not.toContain("alert(1)");
    expect(out).toContain("ok");
  });

  test("elimina handlers on* en cualquier etiqueta", () => {
    const out = sanitizeHTML(`<img src="x" onerror="alert(1)"><a href="/x" onclick="alert(2)">l</a>`);
    expect(out).not.toContain("onerror");
    expect(out).not.toContain("onclick");
    expect(out).not.toContain("alert");
  });

  test("bloquea javascript: en href/src", () => {
    const out = sanitizeHTML(`<a href="javascript:alert(1)">x</a><img src="javascript:alert(2)">`);
    expect(out).not.toContain("javascript:");
    expect(out).not.toContain("alert");
  });

  test("conserva href seguro y añade rel/target", () => {
    const out = sanitizeHTML(`<a href="https://example.com">x</a>`);
    expect(out).toContain("https://example.com");
    expect(out).toContain("noopener");
  });

  test("los inputs de checkbox quedan deshabilitados", () => {
    const out = sanitizeHTML(`<input type="checkbox" checked><input type="text" value="x">`);
    expect(out).toContain("checkbox");
    expect(out).toContain("disabled");
    expect(out).not.toContain('type="text"');
  });

  test("las etiquetas desconocidas se deshijan pero conservan el texto", () => {
    const out = sanitizeHTML(`<foo data-x="1">texto</foo><p>ok</p>`);
    expect(out).not.toContain("foo");
    expect(out).toContain("texto");
    expect(out).toContain("ok");
  });

  test("iframe/object/script se eliminan con todo su contenido", () => {
    const out = sanitizeHTML(`<iframe src="evil">x</iframe><object data="y">z</object><p>ok</p>`);
    expect(out).not.toContain("iframe");
    expect(out).not.toContain("object");
    expect(out).not.toContain("evil");
    expect(out).toContain("ok");
  });

  test("style y class peligrosos se eliminan fuera de la allowlist", () => {
    const out = sanitizeHTML(`<p style="position:fixed" class="x">t</p><div onclick="x" style="y">d</div>`);
    expect(out).not.toContain("style=");
    expect(out).not.toContain("position:fixed");
  });
});
