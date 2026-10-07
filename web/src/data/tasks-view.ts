import { signal } from "@elurjs/core";

// Vista de tareas compartida entre el header (topbar) y TasksPage.
// El header navega y setea la vista; las pestañas internas también.
export type TasksView = "kanban" | "tabla" | "calendario";

const initial = (): TasksView => {
  const q = new URLSearchParams(window.location.hash.split("?")[1] ?? "");
  const v = q.get("view");
  return v === "tabla" || v === "calendario" ? v : "kanban";
};

export const tasksView = signal<TasksView>(initial());

export function setTasksView(v: TasksView): void {
  tasksView.value = v;
}
