// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Sanitizador HTML por allowlist sobre el DOM real (no regex): el
// Markdown del usuario puede contener HTML crudo y marked no sanitiza.
// Un doc compartido por /p/:token se renderiza en el MISMO origen que
// la app autenticada — sin esto hay robo de sesión (XSS persistente).

const ALLOWED_TAGS = new Set([
  "A", "ABBR", "B", "BLOCKQUOTE", "BR", "CAPTION", "CODE", "DD", "DEL", "DETAILS",
  "DIV", "DL", "DT", "EM", "FIGCAPTION", "FIGURE", "H1", "H2", "H3", "H4", "H5", "H6",
  "HR", "I", "IMG", "INPUT", "INS", "KBD", "LI", "MARK", "OL", "P", "PRE", "Q",
  "S", "SAMP", "SECTION", "SMALL", "SPAN", "STRONG", "SUB", "SUMMARY", "SUP",
  "TABLE", "TBODY", "TD", "TFOOT", "TH", "THEAD", "TR", "U", "UL", "VAR",
  "IFRAME", "AUDIO", "VIDEO", "SOURCE",
]);

const ALLOWED_ATTR: Record<string, Set<string>> = {
  A: new Set(["href", "title"]),
  IMG: new Set(["src", "alt", "title", "width", "height"]),
  INPUT: new Set(["type", "checked", "disabled"]),
  TD: new Set(["colspan", "rowspan", "align"]),
  TH: new Set(["colspan", "rowspan", "align", "scope"]),
  OL: new Set(["start"]),
  DETAILS: new Set(["open"]),
  DIV: new Set(["class"]),
  SPAN: new Set(["class"]),
  P: new Set(["class"]),
  CODE: new Set(["class"]),
  PRE: new Set(["class"]),
  H1: new Set(["id"]),
  H2: new Set(["id"]),
  H3: new Set(["id"]),
  H4: new Set(["id"]),
  H5: new Set(["id"]),
  H6: new Set(["id"]),
  LI: new Set(["class"]),
  UL: new Set(["class"]),
  SECTION: new Set(["class"]),
  IFRAME: new Set(["src", "width", "height", "allowfullscreen", "loading", "title"]),
  AUDIO: new Set(["src", "controls", "preload"]),
  VIDEO: new Set(["src", "controls", "preload", "width", "height", "poster"]),
  SOURCE: new Set(["src", "type"]),
};

const URL_ATTRS = new Set(["href", "src"]);

// dominios seguros para iframes (embeds de video/audio)
const SAFE_EMBED_HOSTS = new Set([
  "youtube.com", "www.youtube.com", "youtu.be", "player.vimeo.com",
  "vimeo.com", "www.youtube-nocookie.com", "open.spotify.com",
  "w.soundcloud.com", "player.twitch.tv", "embed.music.apple.com",
]);

// estas etiquetas se eliminan con todo su contenido (texto incluido):
// su texto es activo (JS/CSS) y no debe sobrevivir al deshijar
const DROP_CONTENT = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "TEMPLATE", "OBJECT", "EMBED", "SVG", "MATH"]);

function safeURL(raw: string): string | null {
  const v = raw.trim().replace(/[\u0000-\u001f\u007f]/g, "").toLowerCase();
  if (v.startsWith("javascript:") || v.startsWith("vbscript:") || v.startsWith("data:text/html")) {
    return null;
  }
  return raw;
}

function safeEmbedSrc(raw: string): boolean {
  try {
    const u = new URL(raw);
    return u.protocol === "https:" && SAFE_EMBED_HOSTS.has(u.hostname);
  } catch {
    return false;
  }
}

function scrub(el: Element): void {
  const tag = el.tagName.toUpperCase();
  if (DROP_CONTENT.has(tag)) {
    el.remove();
    return;
  }
  if (!ALLOWED_TAGS.has(tag)) {
    // el elemento se desecha pero los hijos textuales se conservan
    el.replaceWith(...Array.from(el.childNodes));
    return;
  }
  for (const attr of Array.from(el.attributes)) {
    const name = attr.name.toLowerCase();
    const allow = ALLOWED_ATTR[tag];
    if (!allow || !allow.has(name)) {
      el.removeAttribute(attr.name);
      continue;
    }
    if (name.startsWith("on")) {
      el.removeAttribute(attr.name);
      continue;
    }
    if (URL_ATTRS.has(name)) {
      const url = safeURL(attr.value);
      if (url == null) el.removeAttribute(attr.name);
      else el.setAttribute(attr.name, url);
    }
  }
  if (tag === "A") {
    el.setAttribute("rel", "noopener noreferrer nofollow");
    el.setAttribute("target", "_blank");
  }
  if (tag === "INPUT") {
    const type = (el.getAttribute("type") ?? "").toLowerCase();
    if (type !== "checkbox") el.removeAttribute("type");
    el.setAttribute("disabled", "");
  }
  if (tag === "IFRAME") {
    const src = el.getAttribute("src") ?? "";
    if (!safeEmbedSrc(src)) {
      el.remove();
      return;
    }
  }
  if (tag === "AUDIO" || tag === "VIDEO") {
    const src = el.getAttribute("src") ?? "";
    if (src && !safeURL(src)) {
      el.removeAttribute("src");
    }
  }
  for (const child of Array.from(el.children)) scrub(child);
}

// sanitizeHTML purifica un fragmento HTML (p. ej. la salida de marked)
// dejando solo las etiquetas/atributos de un Markdown inofensivo.
export function sanitizeHTML(html: string): string {
  if (typeof document === "undefined") return "";
  const tpl = document.createElement("template");
  tpl.innerHTML = html;
  for (const child of Array.from(tpl.content.children)) scrub(child);
  return tpl.innerHTML;
}
