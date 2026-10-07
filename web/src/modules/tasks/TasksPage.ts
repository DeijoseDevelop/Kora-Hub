import { ElurComponent, html, signal, type ElurTemplate } from "@elurjs/core";
import { localTasks } from "../../data/store";
import { activeWs } from "../../data/workspace";
import { tasksView } from "../../data/tasks-view";
import { toggleTaskLocal, quickAddLocal, setTaskStateLocal, setTaskTitleLocal } from "../../data/mutations";
import { currentRole } from "../../api/role";
import { formatDate, isOverdue, showPrompt, showToast } from "../../ui/kit";
import { t as tr, tList, locale } from "../../i18n";
import { viewsApi, type SavedView } from "../../api/client";
import type { LocalTask } from "../../sync/local";

// Proyecciones del índice local (kanban/tabla/calendario) — 100% offline.
// La vista es compartida con el header (data/tasks-view): los botones
// del topbar la setean al navegar (antes se leía una sola vez al cargar
// el módulo y el calendario abría el kanban, etc.).
const tableParams = signal<[string, string]>(["", "0"]);
const calRange = signal<[string, string]>(monthRange());

let quickAddText = "";

// ------------------------- Vistas guardadas -------------------------
// D1: filtros serializados por workspace. Se sirven por API y se
// cachean en localStorage para que apliquen también offline (aplicar
// una vista es filtrado client-side sobre el mirror local).
const savedViews = signal<SavedView[]>([]);

function viewsCacheKey(): string {
  return `hub:views:${activeWs.value ?? ""}`;
}

async function loadViews(): Promise<void> {
  const ws = activeWs.value;
  if (!ws) return;
  try {
    const cached = localStorage.getItem(viewsCacheKey());
    if (cached) savedViews.value = JSON.parse(cached) as SavedView[];
  } catch { /* cache corrupta: se ignora */ }
  try {
    const res = await viewsApi.list(ws);
    savedViews.value = res.views;
    localStorage.setItem(viewsCacheKey(), JSON.stringify(res.views));
  } catch { /* offline: se queda la copia cacheada */ }
}

function currentFilters(): Record<string, string> {
  return {
    vista: tasksView.value,
    proyecto: tableParams.value[0],
    done: tableParams.value[1],
  };
}

function applyView(v: SavedView): void {
  try {
    const f = JSON.parse(v.filters) as Record<string, string>;
    if (f.vista === "kanban" || f.vista === "tabla" || f.vista === "calendario") {
      tasksView.value = f.vista;
    }
    tableParams.value = [f.proyecto ?? "", f.done ?? "0"];
  } catch { /* filters corruptas: no-op */ }
}

async function saveCurrentView(): Promise<void> {
  const ws = activeWs.value;
  const name = await showPrompt(tr("tasks.save_view_title"), tr("tasks.save_view_ph"));
  if (!ws || !name?.trim()) return;
  try {
    await viewsApi.create(ws, name.trim(), currentFilters());
    await loadViews();
    showToast(tr("tasks.view_saved"));
  } catch (e) {
    showToast((e as Error).message);
  }
}

async function removeView(id: string): Promise<void> {
  const ws = activeWs.value;
  if (!ws) return;
  try {
    await viewsApi.remove(id, ws);
    await loadViews();
    showToast(tr("tasks.view_deleted"));
  } catch (e) {
    showToast((e as Error).message);
  }
}

function viewsBar(): ElurTemplate {
  return html`
    <div class="views-bar">
      <select @change=${(ev: Event) => {
      const v = savedViews.value.find((x) => x.id === (ev.target as HTMLSelectElement).value);
      if (v) applyView(v);
      (ev.target as HTMLSelectElement).value = "";
    }}>
        <option value="">${() => tr("tasks.views_saved")}</option>
        ${() => savedViews.value.map((v) => html`<option value=${v.id}>${v.name}</option>`)}
      </select>
      <button class="btn ghost" @click=${() => void saveCurrentView()}>${() => tr("tasks.save_view")}</button>
      <select @change=${(ev: Event) => {
      const id = (ev.target as HTMLSelectElement).value;
      if (id) void removeView(id);
      (ev.target as HTMLSelectElement).value = "";
    }}>
        <option value="">${() => tr("tasks.delete_view")}</option>
        ${() => savedViews.value.map((v) => html`<option value=${v.id}>${v.name}</option>`)}
      </select>
    </div>
  `;
}

function monthRange(): [string, string] {
  const now = new Date();
  const y = now.getFullYear();
  const m = now.getMonth();
  return [
    `${y}-${String(m + 1).padStart(2, "0")}-01`,
    `${y}-${String(m + 1).padStart(2, "0")}-${String(new Date(y, m + 1, 0).getDate()).padStart(2, "0")}`,
  ];
}

function shiftMonth(delta: number): void {
  const base = new Date(calRange.value[0] + "T12:00:00");
  const y = base.getFullYear();
  const m = base.getMonth() + delta;
  const nm = new Date(y, ((m % 12) + 12) % 12, 1);
  const first = `${nm.getFullYear()}-${String(nm.getMonth() + 1).padStart(2, "0")}-01`;
  const last = `${nm.getFullYear()}-${String(nm.getMonth() + 1).padStart(2, "0")}-${String(new Date(nm.getFullYear(), nm.getMonth() + 1, 0).getDate()).padStart(2, "0")}`;
  calRange.value = [first, last];
}

function openDoc(docId: string): void {
  window.dispatchEvent(new CustomEvent("hub:open-doc", { detail: docId }));
}

async function toggle(task: LocalTask): Promise<void> {
  const role = await currentRole();
  if (role === "viewer") {
    showToast(tr("tasks.viewer_ro"));
    return;
  }
  try {
    await toggleTaskLocal(task);
    showToast(task.done ? tr("tasks.reopened") : tr("tasks.completed_ok"));
  } catch (e) {
    showToast((e as Error).message);
  }
}

async function quickAdd(): Promise<void> {
  const raw = quickAddText.trim();
  if (!raw) return;
  const role = await currentRole();
  if (role === "viewer") {
    showToast(tr("tasks.viewer_ro"));
    return;
  }
  try {
    const tk = await quickAddLocal(raw);
    showToast(tk ? tr("tasks.created", { title: tk.title }) : tr("tasks.unparseable"));
    quickAddText = "";
  } catch (e) {
    showToast((e as Error).message);
  }
}

// isBlocked resuelve la dependencia ^blocked-by: contra el mirror
// local: la tarea esta bloqueada si su bloqueante existe y no esta hecha.
function isBlocked(t: LocalTask): boolean {
  if (!t.blockedBy) return false;
  const blocker = localTasks.value.find(
    (x) => x.workspaceId === t.workspaceId && x.taskUid === t.blockedBy,
  );
  return blocker != null && !blocker.done;
}

function taskBadges(t: LocalTask): ElurTemplate {
  return html`
    ${t.dueDate
      ? html`<span class=${"badge" + " date" + (isOverdue(t.dueDate) ? " overdue" : "")}>${formatDate(t.dueDate)}</span>`
      : ""}
    ${t.recur ? html`<span class="badge recur" title=${() => tr("tasks.recurring")}>↻ ${t.recur}</span>` : ""}
    ${isBlocked(t) ? html`<span class="badge blocked" title=${() => tr("tasks.blocked_by", { id: t.blockedBy ?? "" })}>${tr("tasks.blocked")}</span>` : ""}
    ${t.project ? html`<span class="badge project">${"@" + t.project}</span>` : ""}
    ${t.priority ? html`<span class=${"badge priority-" + t.priority}>${t.priority}</span>` : ""}
  `;
}

// ---------------------------- Kanban ----------------------------

function kanbanColumn(label: string, dot: string, state: " " | "x" | "~", getter: () => LocalTask[]): ElurTemplate {
  return html`
    <div class=${"kanban-col" + " " + dot}>
      <div class="kanban-col-header">
        <span><span class="dot"></span>${label}</span>
        <span class="count">${() => getter().length}</span>
      </div>
      <div class="kanban-col-body" data-state=${state}
        @dragover=${(ev: DragEvent) => {
      ev.preventDefault();
      ev.dataTransfer!.dropEffect = "move";
      (ev.currentTarget as HTMLElement).classList.add("drag-over");
    }}
        @dragleave=${(ev: DragEvent) => (ev.currentTarget as HTMLElement).classList.remove("drag-over")}
        @drop=${(ev: DragEvent) => {
      ev.preventDefault();
      (ev.currentTarget as HTMLElement).classList.remove("drag-over");
      const id = ev.dataTransfer?.getData("text/plain");
      const target = localTasks.value.find((t) => t.id === id);
      // semántica por columna: todo→[ ] doing→[~] done→[x] (no un flip)
      if (target) {
        const cur: " " | "x" | "~" = target.done ? "x" : target.inProgress ? "~" : " ";
        if (cur !== state) void setTaskStateLocal(target, state);
      }
    }}>
        ${() => {
      const items = getter();
      if (items.length === 0) {
        return html`<div class="kanban-empty">${() => tr("tasks.col_empty")}</div>`;
      }
      // agrupar: sub-tareas (depth>0) se muestran dentro de su padre
      const top = items.filter((t) => !t.depth);
      const childrenOf = (parentLine: number) =>
        items.filter((t) => t.depth > 0 && t.parentLine === parentLine);
      return top.map((t) => {
        const subs = childrenOf(t.lineNo);
        const subDone = subs.filter((s) => s.done).length;
        return html`
            <div class=${"task-card" + (t.done ? " done" : "") + (subs.length ? " has-subs" : "")} draggable="true"
              @dragstart=${(ev: DragEvent) => {
          (ev.currentTarget as HTMLElement).classList.add("dragging");
          ev.dataTransfer?.setData("text/plain", t.id);
          ev.dataTransfer!.effectAllowed = "move";
        }}
              @dragend=${(ev: DragEvent) => (ev.currentTarget as HTMLElement).classList.remove("dragging")}>
              <div class="task-text">${t.title}</div>
              ${subs.length > 0 ? html`
                <div class="sub-tasks">
                  <div class="sub-progress">
                    <div class="sub-progress-bar"><div class="sub-progress-fill" style=${"width:" + Math.round((subDone / Math.max(1, subs.length)) * 100) + "%"}></div></div>
                    <span class="sub-count">${subDone}/${subs.length}</span>
                  </div>
                  ${subs.map((s) => html`
                    <div class=${"sub-task" + (s.done ? " done" : "")} draggable="true"
                      @dragstart=${(ev: DragEvent) => {
                  ev.stopPropagation();
                  (ev.currentTarget as HTMLElement).classList.add("dragging");
                  ev.dataTransfer?.setData("text/plain", s.id);
                  ev.dataTransfer!.effectAllowed = "move";
                }}
                      @dragend=${(ev: DragEvent) => (ev.currentTarget as HTMLElement).classList.remove("dragging")}>
                      <span class=${"mini-checkbox" + (s.done ? " checked" : "")} @click=${() => void toggle(s)}></span>
                      <span class="sub-task-text">${s.title}</span>
                    </div>`)}
                </div>` : ""}
              <div class="task-meta">
                <div class="task-badges">${taskBadges(t)}</div>
                <span class="source-doc" title=${() => tr("tasks.open_doc")}
                  @click=${(ev: MouseEvent) => {
          ev.stopPropagation();
          openDoc(t.docId);
        }}>
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
                  ${() => tr("tasks.open_doc")}
                </span>
              </div>
            </div>`;
      });
    }}
      </div>
    </div>
  `;
}

function kanbanView(): ElurTemplate {
  return html`
    <div class="kanban-toolbar">
      <div class="quick-add">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color: var(--text-faint)"><path d="M12 5v14M5 12h14"/></svg>
        <input type="text" placeholder=${() => tr("tasks.quick_ph")}
          value=${() => quickAddText}
          @input=${(ev: Event) => (quickAddText = (ev.target as HTMLInputElement).value)}
          @keydown=${(ev: KeyboardEvent) => { if (ev.key === "Enter") void quickAdd(); }} />
        <button class="quick-add-btn" @click=${() => void quickAdd()}>${() => tr("tasks.add")}</button>
      </div>
      <span class="quick-add-hint">${() => tr("tasks.quick_hint")}</span>
      <select class="group-select" value=${() => cycleFilter.value}
        @change=${(ev: Event) => (cycleFilter.value = (ev.target as HTMLSelectElement).value)}>
        <option value="">${() => tr("tasks.all_cycles")}</option>
        ${() => cycles().map((c) => html`<option value=${c}>${"🔄 " + c}</option>`)}
      </select>
      ${() => cycleFilter.value ? html`
        <div class="cycle-progress">
          <div class="cycle-progress-bar"><div class="cycle-progress-fill" style=${"width:" + Math.round((cycleProgress().done / Math.max(1, cycleProgress().total)) * 100) + "%"}></div></div>
          <span class="cycle-progress-text">${cycleProgress().done}/${cycleProgress().total} ${() => tr("tasks.cycle_done")}</span>
        </div>` : ""}
      <select class="group-select" value=${() => groupBy.value}
        @change=${(ev: Event) => (groupBy.value = (ev.target as HTMLSelectElement).value as typeof groupBy.value)}>
        <option value="">${() => tr("tasks.group_none")}</option>
        <option value="project">${() => tr("tasks.group_project")}</option>
        <option value="assignee">${() => tr("tasks.group_assignee")}</option>
        <option value="priority">${() => tr("tasks.group_priority")}</option>
      </select>
    </div>
    ${() => {
      const ws = activeWs.value;
      const all = tasksInCycle();
      const gb = groupBy.value;
      if (!gb) {
        return html`<div class="kanban-board">
          ${kanbanColumn(tr("tasks.todo"), "todo", " ", () => all.filter((t) => !t.done && !t.inProgress))}
          ${kanbanColumn(tr("tasks.doing"), "doing", "~", () => all.filter((t) => !t.done && t.inProgress))}
          ${kanbanColumn(tr("tasks.done"), "done", "x", () => all.filter((t) => t.done))}
        </div>`;
      }
      const groups = new Map<string, typeof all>();
      for (const t of all) {
        const key = gb === "project" ? (t.project ?? "—") : gb === "assignee" ? (t.assignee ?? "—") : (t.priority ?? "—");
        if (!groups.has(key)) groups.set(key, []);
        groups.get(key)!.push(t);
      }
      return html`<div class="kanban-grouped">
        ${[...groups.entries()].map(([key, tasks]) => html`
          <div class="kanban-group">
            <h3 class="kanban-group-label">${gb === "project" ? "@" + key : key} <span class="muted">(${tasks.length})</span></h3>
            <div class="kanban-board">
              ${kanbanColumn(tr("tasks.todo"), "todo", " ", () => tasks.filter((t) => !t.done && !t.inProgress))}
              ${kanbanColumn(tr("tasks.doing"), "doing", "~", () => tasks.filter((t) => !t.done && t.inProgress))}
              ${kanbanColumn(tr("tasks.done"), "done", "x", () => tasks.filter((t) => t.done))}
            </div>
          </div>`)}
      </div>`;
    }}
    <p class="muted" style="padding: 0 20px 14px">${() => tr("tasks.drag_hint")}</p>
  `;
}

// ---------------------------- Tabla ----------------------------

const tableSort = signal<{ col: string; dir: 1 | -1 }>({ col: "dueDate", dir: 1 });
const selectedIds = signal<Set<string>>(new Set());
const groupBy = signal<"" | "project" | "assignee" | "priority">("");
const editingCell = signal<{ id: string; field: string } | null>(null);
const cycleFilter = signal<string>("");

// cycles extrae los nombres de ciclo de las tags de las tareas (+cycle:X)
function cycles(): string[] {
  const set = new Set<string>();
  for (const t of localTasks.value) {
    if (t.workspaceId !== activeWs.value) continue;
    for (const tag of t.tags ?? []) {
      if (tag.startsWith("cycle:")) set.add(tag.slice(6));
    }
  }
  return [...set].sort();
}

// tasksInCycle filtra por ciclo seleccionado (vacío = todos)
function tasksInCycle(): LocalTask[] {
  const ws = activeWs.value;
  const c = cycleFilter.value;
  return localTasks.value.filter((t) => {
    if (t.workspaceId !== ws) return false;
    if (c && !(t.tags ?? []).includes("cycle:" + c)) return false;
    return true;
  });
}

// cycleProgress devuelve {done, total} de las tareas del ciclo activo
function cycleProgress(): { done: number; total: number } {
  const all = tasksInCycle();
  return { done: all.filter((t) => t.done).length, total: all.length };
}

function tablaView(): ElurTemplate {
  const filtered = () => {
    const [proyecto, done] = tableParams.value;
    return localTasks.value.filter((t) => {
      if (t.workspaceId !== activeWs.value) return false;
      if (proyecto && t.project !== proyecto) return false;
      if (done === "1" ? !t.done : t.done) return false;
      return true;
    });
  };
  const sorted = () => {
    const s = tableSort.value;
    const rows = [...filtered()];
    rows.sort((a, b) => {
      let va: string | number = "", vb: string | number = "";
      switch (s.col) {
        case "title": va = a.title.toLowerCase(); vb = b.title.toLowerCase(); break;
        case "dueDate": va = a.dueDate ?? "zzz"; vb = b.dueDate ?? "zzz"; break;
        case "project": va = a.project ?? ""; vb = b.project ?? ""; break;
        case "priority": va = a.priority ?? ""; vb = b.priority ?? ""; break;
        default: va = a.title; vb = b.title;
      }
      return (va < vb ? -1 : va > vb ? 1 : 0) * s.dir;
    });
    return rows;
  };
  const sortBy = (col: string) => {
    tableSort.value = {
      col,
      dir: tableSort.value.col === col ? ((tableSort.value.dir === 1 ? -1 : 1) as 1 | -1) : 1,
    };
  };
  const toggleSelect = (id: string) => {
    const s = new Set(selectedIds.value);
    s.has(id) ? s.delete(id) : s.add(id);
    selectedIds.value = s;
  };
  const selectAll = () => {
    const rows = sorted();
    const s = selectedIds.value;
    selectedIds.value = s.size === rows.length ? new Set() : new Set(rows.map((t) => t.id));
  };
  const bulkDone = (done: boolean) => {
    const s = selectedIds.value;
    for (const t of sorted()) {
      if (s.has(t.id) && t.done !== done) void toggle(t);
    }
    selectedIds.value = new Set();
  };
  const sortIcon = (col: string) =>
    tableSort.value.col === col ? (tableSort.value.dir === 1 ? "↑" : "↓") : "";

  return html`
    <div class="list-toolbar">
      <select value=${() => tableParams.value[0]}
        @change=${(ev: Event) => {
      tableParams.value = [(ev.target as HTMLSelectElement).value, tableParams.value[1]];
    }}>
        <option value="">${() => tr("tasks.all_projects")}</option>
        ${() =>
      [...new Set(localTasks.value.filter((t) => t.workspaceId === activeWs.value).map((t) => t.project).filter((p): p is string => !!p))].sort().map((p) => html`<option value=${p}>@${p}</option>`)}
      </select>
      <select value=${() => tableParams.value[1]}
        @change=${(ev: Event) => {
      tableParams.value = [tableParams.value[0], (ev.target as HTMLSelectElement).value];
    }}>
        <option value="0">${() => tr("tasks.pending")}</option>
        <option value="1">${() => tr("tasks.completed")}</option>
      </select>
      ${() => selectedIds.value.size > 0 ? html`
        <div class="bulk-bar">
          <span class="bulk-count">${selectedIds.value.size} ${() => tr("tasks.selected")}</span>
          <button class="btn sm ghost" @click=${() => bulkDone(true)}>${() => tr("tasks.bulk_done")}</button>
          <button class="btn sm ghost" @click=${() => bulkDone(false)}>${() => tr("tasks.bulk_undone")}</button>
          <button class="btn sm link" @click=${() => (selectedIds.value = new Set())}>${() => tr("tasks.bulk_clear")}</button>
        </div>` : ""}
    </div>
    <div class="list-table-wrap">
      <table class="list-table">
        <thead>
          <tr>
            <th style="width: 30px"><button class="mini-checkbox ${() => selectedIds.value.size === sorted().length && sorted().length > 0 ? "checked" : ""}" @click=${selectAll}></button></th>
            <th class="sortable" @click=${() => sortBy("title")}>${() => tr("tasks.col_task")} ${sortIcon("title")}</th>
            <th class="sortable" @click=${() => sortBy("dueDate")}>${() => tr("tasks.col_date")} ${sortIcon("dueDate")}</th>
            <th class="sortable" @click=${() => sortBy("project")}>${() => tr("tasks.col_project")} ${sortIcon("project")}</th>
            <th class="sortable" @click=${() => sortBy("priority")}>${() => tr("tasks.col_priority")} ${sortIcon("priority")}</th>
          </tr>
        </thead>
        <tbody>
          ${() =>
      sorted().map((t) => html`
                <tr class=${selectedIds.value.has(t.id) ? "selected-row" : ""}>
                  <td>
                    <button class=${"mini-checkbox" + (selectedIds.value.has(t.id) ? " checked" : "")} aria-label="select"
                      @click=${() => toggleSelect(t.id)}></button>
                  </td>
                  <td>
                    <button class=${"mini-checkbox" + (t.done ? " checked" : "")} aria-label="completar"
                      @click=${() => void toggle(t)}></button>
                    ${editingCell.value?.id === t.id && editingCell.value?.field === "title"
          ? html`<input class="inline-edit" value=${t.title}
                        @blur=${(ev: Event) => {
                      const val = (ev.target as HTMLInputElement).value.trim();
                      if (val && val !== t.title) void setTaskTitleLocal(t, val);
                      editingCell.value = null;
                    }}
                        @keydown=${(ev: KeyboardEvent) => {
                      if (ev.key === "Enter") (ev.target as HTMLInputElement).blur();
                      if (ev.key === "Escape") editingCell.value = null;
                    }} autofocus />`
          : html`<span class=${t.done ? "task-text-done" : ""} style="cursor:text"
                      title=${() => tr("tasks.click_edit")}
                      @dblclick=${() => (editingCell.value = { id: t.id, field: "title" })}>${t.title}</span>`}
                  </td>
                  <td>${t.dueDate ? html`<span class=${"badge" + " date" + (isOverdue(t.dueDate) ? " overdue" : "")}>${formatDate(t.dueDate)}</span>` : html`<span class="faint">—</span>`}</td>
                  <td>${t.project ? html`<span class="badge project">${"@" + t.project}</span>` : html`<span class="faint">—</span>`}</td>
                  <td>${t.priority ? html`<span class=${"badge priority-" + t.priority}>${t.priority}</span>` : ""}</td>
                </tr>`)}
          ${() =>
      sorted().length === 0
        ? html`<tr><td colspan="5" class="empty-state">${() => tr("tasks.empty_filter")}</td></tr>`
        : ""}
        </tbody>
      </table>
    </div>
  `;
}

// -------------------------- Calendario --------------------------

function calendarioView(): ElurTemplate {
  const range = calRange.value;
  const y = Number(range[0].slice(0, 4));
  const m = Number(range[0].slice(5, 7)) - 1;
  const firstDay = new Date(y, m, 1);
  const lastDay = new Date(y, m + 1, 0);
  let startOffset = firstDay.getDay() - 1;
  if (startOffset < 0) startOffset = 6;
  const prevLast = new Date(y, m, 0).getDate();
  const isoToday = new Date().toISOString().split("T")[0];
  const dayNames = tList("kit.weekdays_short");

  const cells: Array<{ date: string; isToday: boolean; other: boolean }> = [];
  for (let i = startOffset - 1; i >= 0; i--) {
    cells.push({ date: `${y}-${String(m).padStart(2, "0")}-${String(prevLast - i).padStart(2, "0")}`, isToday: false, other: true });
  }
  for (let d = 1; d <= lastDay.getDate(); d++) {
    const dateStr = `${y}-${String(m + 1).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
    cells.push({ date: dateStr, isToday: dateStr === isoToday, other: false });
  }
  const remaining = (7 - ((startOffset + lastDay.getDate()) % 7)) % 7;
  for (let i = 1; i <= remaining; i++) {
    cells.push({ date: `${y}-${String(m + 2).padStart(2, "0")}-${String(i).padStart(2, "0")}`, isToday: false, other: true });
  }

  return html`
    <div class="view-calendar">
      <div class="calendar-header">
        <h3>${() => new Date(y, m, 1).toLocaleDateString(locale.value, { month: "long", year: "numeric" })}</h3>
        <div class="calendar-nav">
          <button @click=${() => shiftMonth(-1)}>←</button>
          <button @click=${() => (calRange.value = monthRange())}>${() => tr("tasks.today")}</button>
          <button @click=${() => shiftMonth(1)}>→</button>
        </div>
      </div>
      <div class="calendar-grid">
        ${dayNames.map((d) => html`<div class="calendar-day-name">${d}</div>`)}
        ${cells.map((cell) => html`
          <div class=${"calendar-cell" + (cell.other ? " other-month" : "") + (cell.isToday ? " today" : "")}>
            <div class="day-number">${Number(cell.date.slice(8, 10))}</div>
            ${() =>
      localTasks.value
        .filter((t) => t.workspaceId === activeWs.value && t.dueDate === cell.date && !cell.other)
        .map((t) => html`<button class=${"cal-task" + " " + (t.priority ?? "") + (t.done ? " done" : "")}
                  title=${t.title} @click=${() => openDoc(t.docId)}>${t.title}</button>`)}
          </div>`)}
      </div>
    </div>
  `;
}

// ----------------------------- Page -----------------------------

export class TasksPage extends ElurComponent {
  onMount(): void {
    void loadViews();
  }

  render(): ElurTemplate {
    return html`
      <div class="page">
        <div class="page-header">
          <h2>${() => tr("tasks.title")}</h2>
          <div class="tabs">
            ${(["kanban", "tabla", "calendario"] as const).map(
      (v) => html`<button class=${() => "tab" + (tasksView.value === v ? " active" : "")}
                @click=${() => (tasksView.value = v)}>${() => tr({ kanban: "app.nav.kanban", tabla: "app.nav.table", calendario: "app.nav.calendar" }[v])}</button>`,
    )}
          </div>
        </div>
        ${() => viewsBar()}
        ${() =>
        tasksView.value === "kanban"
          ? kanbanView()
          : tasksView.value === "tabla"
            ? tablaView()
            : calendarioView()}
      </div>
    `;
  }
}
