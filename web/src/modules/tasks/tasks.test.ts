import assert from "node:assert";
import { render } from "@elurjs/core-testing";
import { test } from "vitest";
import { TasksPage } from "./TasksPage";

test("renderiza el tablero con las tres columnas y quick add", async () => {
  localStorage.removeItem("hub:token");
  const { container } = render(new TasksPage() as never);
  await new Promise((r) => setTimeout(r, 300));
  assert.strictEqual(container.querySelectorAll(".kanban-col").length, 3);
  assert.ok(container.querySelector(".quick-add input"));
});
