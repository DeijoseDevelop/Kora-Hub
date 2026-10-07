// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Capa i18n (decisión D4): signal de locale + t() con fallback a ES.
// Regla de uso: toda cadena visible va por t() dentro de un binding
// reactivo ${() => t("clave")} para que el switch re-renderice.
// Los catálogos se cargan eager (son ~4 KB; la división no compensa).
import { signal } from "@elurjs/core";
import { es } from "./es";
import { en } from "./en";

export type Locale = "es" | "en";

export const catalogs: Record<Locale, Record<string, string>> = { es, en };

const STORAGE_KEY = "hub:locale";

function detect(): Locale {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "es" || saved === "en") return saved;
  } catch {
    /* sin storage (tests) */
  }
  const nav = typeof navigator === "undefined" ? "es" : navigator.language;
  return nav.toLowerCase().startsWith("en") ? "en" : "es";
}

export const locale = signal<Locale>(detect());

export function setLocale(l: Locale): void {
  locale.value = l;
  try {
    localStorage.setItem(STORAGE_KEY, l);
  } catch {
    /* sin storage */
  }
  if (typeof document !== "undefined") document.documentElement.lang = l;
}

// t traduce una clave con variables {x}. Fallback: ES -> la clave misma
// (una clave visible en la UI es bug ruidoso, no crash silencioso).
export function t(key: string, vars?: Record<string, string | number>): string {
  let s = catalogs[locale.value][key] ?? es[key] ?? key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) s = s.split("{" + k + "}").join(String(v));
  }
  return s;
}

// list devuelve un catálogo como array (meses, días de semana).
export function tList(key: string): string[] {
  return t(key).split(",");
}
