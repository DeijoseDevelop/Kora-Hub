// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import assert from "node:assert";
import { describe, test } from "vitest";
import { catalogs, locale, setLocale, t } from "./index";
import { es } from "./es";
import { en } from "./en";

describe("i18n", () => {
  test("los catálogos ES y EN tienen exactamente las mismas claves", () => {
    const esKeys = Object.keys(es).sort();
    const enKeys = Object.keys(en).sort();
    assert.deepStrictEqual(enKeys, esKeys);
    assert.equal(catalogs.es, es);
    assert.equal(catalogs.en, en);
  });

  test("t() traduce, interpola variables y cae a ES/clave", () => {
    setLocale("es");
    assert.equal(t("settings.title"), "Ajustes");
    assert.equal(t("tasks.created", { title: "llamar" }), "Tarea creada: llamar");

    setLocale("en");
    assert.equal(t("settings.title"), "Settings");
    assert.equal(t("tasks.created", { title: "call" }), "Task created: call");

    // clave desconocida: la clave misma (bug visible, nunca crash)
    assert.equal(t("no.existe.clave"), "no.existe.clave");
    setLocale("es");
  });

  test("setLocale persiste en localStorage", () => {
    setLocale("en");
    assert.equal(locale.value, "en");
    assert.equal(localStorage.getItem("hub:locale"), "en");
    setLocale("es");
    assert.equal(localStorage.getItem("hub:locale"), "es");
  });
});
