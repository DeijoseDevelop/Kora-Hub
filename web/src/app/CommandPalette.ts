import { NixComponent, html, signal, type NixTemplate } from "@deijose/nix-js";
import { router } from "../router";
import { localDocs, localTasks } from "../data/store";
import { activeWs } from "../data/workspace";
import { quickAddLocal } from "../data/mutations";
import { currentRole } from "../api/role";
import { fuzzyMatch, showPrompt, showToast } from "../ui/kit";
import { t } from "../i18n";

type PaletteKind = "action" | "doc" | "task";

interface PaletteItem {
  kind: PaletteKind;
  label: string;
  sub: string;
  action: () => void;
}

// Command palette (Ctrl+K) local-first: busca en el índice local
// (docs + tareas) — funciona sin conexión.
export class CommandPalette extends NixComponent {
  private open = signal(false);
  private query = signal("");
  private selected = 0;
  private items = signal<PaletteItem[]>([]);

  toggle(): void {
    this.open.value = !this.open.value;
    if (this.open.value) {
      this.query.value = "";
      this.selected = 0;
      this.rebuild();
    }
  }

  close(): void {
    this.open.value = false;
  }

  private rebuild(): void {
    const q = this.query.value.toLowerCase();
    const items: PaletteItem[] = [
      {
        kind: "action",
        label: t("palette.new_task"),
        sub: "Quick Add Magic",
        action: () => {
          void showPrompt(t("palette.new_task_title"), t("palette.new_task_ph")).then((text) => {
            if (!text) return;
            void currentRole().then((role) => {
              if (role === "viewer") {
                showToast(t("tasks.viewer_ro"));
                return;
              }
              quickAddLocal(text)
                .then((tk) => showToast(tk ? t("tasks.created", { title: tk.title }) : t("tasks.unparseable")))
                .catch((e: Error) => showToast(e.message));
            });
          });
        },
      },
      { kind: "action", label: t("palette.new_doc"), sub: t("palette.create_in_ws"), action: () => router.navigate("/docs") },
      { kind: "action", label: t("palette.goto_tasks"), sub: t("palette.tasks_sub"), action: () => router.navigate("/tasks") },
      { kind: "action", label: t("palette.goto_graph"), sub: t("palette.graph_sub"), action: () => router.navigate("/graph") },
    ];

    for (const d of localDocs.value.filter((x) => x.workspaceId === activeWs.value)) {
      items.push({ kind: "doc", label: d.title, sub: d.path, action: () => router.navigate("/docs/" + d.id) });
    }
    for (const tk of localTasks.value.filter((x) => x.workspaceId === activeWs.value).slice(0, 40)) {
      items.push({
        kind: "task",
        label: tk.title,
        sub: tk.done ? t("palette.done") : tk.project ? "@" + tk.project : t("palette.task"),
        action: () => router.navigate("/docs/" + tk.docId),
      });
    }

    this.items.value = q ? items.filter((it) => fuzzyMatch(q, it.label + " " + it.sub)) : items;
    this.selected = 0;
  }

  private keydown(ev: KeyboardEvent): void {
    if (ev.key === "Escape") {
      this.close();
    } else if (ev.key === "ArrowDown") {
      ev.preventDefault();
      this.selected = Math.min(this.items.value.length - 1, this.selected + 1);
    } else if (ev.key === "ArrowUp") {
      ev.preventDefault();
      this.selected = Math.max(0, this.selected - 1);
    } else if (ev.key === "Enter") {
      ev.preventDefault();
      const it = this.items.value[this.selected];
      if (it) {
        it.action();
        this.close();
      }
    }
  }

  render(): NixTemplate {
    const groupLabel: Record<PaletteKind, string> = {
      action: t("palette.actions"), doc: t("palette.docs"), task: t("palette.tasks"),
    };
    const order: PaletteKind[] = ["action", "doc", "task"];
    return html`
      ${() =>
        this.open.value
          ? html`
              <div class="palette-backdrop" @click=${(ev: MouseEvent) => {
              if ((ev.target as HTMLElement).classList.contains("palette-backdrop")) this.close();
            }}>
                <div class="palette">
                  <div class="palette-input">
                    <span class="palette-icon">
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
                    </span>
                    <input autocomplete="off" placeholder=${() => t("palette.ph")}
                      value=${() => this.query.value}
                      @input=${(ev: Event) => {
              this.query.value = (ev.target as HTMLInputElement).value;
              this.rebuild();
            }}
                      @keydown=${(ev: KeyboardEvent) => this.keydown(ev)} />
                  </div>
                  <div class="palette-results">
                    ${() =>
              order.map((kind) => {
                const group = this.items.value.filter((it) => it.kind === kind);
                if (!group.length) return "";
                return html`
                          <div class="palette-section-label">${groupLabel[kind]}</div>
                          ${group.map((it) => html`
                            <div class=${"palette-item" + (this.items.value.indexOf(it) === this.selected ? " selected" : "")}
                              @click=${() => {
                    it.action();
                    this.close();
                  }}
                              @mouseenter=${() => (this.selected = this.items.value.indexOf(it))}>
                              <span class="pi-text">${it.label}<span class="pi-sub"> · ${it.sub}</span></span>
                            </div>`)}`;
              })}
                    ${() =>
              this.items.value.length === 0
                ? html`<div class="palette-empty">${() => t("palette.empty")}</div>`
                : ""}
                  </div>
                  <div class="palette-footer">
                    <span><kbd>↑</kbd><kbd>↓</kbd> ${() => t("palette.nav")}</span>
                    <span><kbd>Enter</kbd> ${() => t("palette.open")}</span>
                    <span><kbd>Esc</kbd> ${() => t("palette.close")}</span>
                  </div>
                </div>
              </div>
            `
          : ""}
    `;
  }
}
