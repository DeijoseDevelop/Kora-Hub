import assert from "node:assert";
import { render } from "@elurjs/core-testing";
import { test } from "vitest";
import { setLocale } from "../../i18n";
import { HomePage } from "./HomePage";

// determinismo: el navegador de test (jsdom) reporta en-US; el producto
// nace en ES — fijamos el locale para que los asserts sean estables.
setLocale("es");

test("muestra los tabs Entrar y Crear cuenta", () => {
  localStorage.removeItem("hub:token");
  const { container } = render(new HomePage() as never);
  const buttons = Array.from(container.querySelectorAll(".tab")).map((b) => b.textContent);
  assert.ok(buttons.includes("Entrar"), "tab Entrar");
  assert.ok(buttons.includes("Crear cuenta"), "tab Crear cuenta");
  assert.ok(container.querySelector('input[type="email"]'));
  assert.ok(container.querySelector('input[type="password"]'));
});

test("cambia al modo registro y muestra el campo de nombre", () => {
  localStorage.removeItem("hub:token");
  const { container } = render(new HomePage() as never);
  const registerTab = Array.from(container.querySelectorAll(".tab")).find((b) => b.textContent === "Crear cuenta") as HTMLButtonElement;
  registerTab.click();
  return new Promise((r) => setTimeout(r, 50)).then(() => {
    assert.ok(container.querySelector('input[type="text"]'), "campo nombre visible");
  });
});
