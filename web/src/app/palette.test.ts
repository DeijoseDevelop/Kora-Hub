import { render } from "@elurjs/core-testing";
import { describe, expect, test } from "vitest";
import { CommandPalette } from "./CommandPalette";

describe("command palette", () => {
  test("se abre y expone la búsqueda", () => {
    const palette = new CommandPalette();
    palette.toggle();
    const { container } = render(palette.render() as never);
    expect(container.querySelector(".palette-input input")).toBeTruthy();
  });

  test("cerrada no renderiza el overlay", () => {
    const palette = new CommandPalette();
    const { container } = render(palette.render() as never);
    expect(container.querySelector(".palette-backdrop")).toBeNull();
  });
});
