import { ElurComponent, RouterView, html, signal, type ElurTemplate } from "@elurjs/core";
import { CommandPalette } from "./CommandPalette";
import { router } from "../router";
import { workspacesApi } from "../api/client";
import { queueLength } from "../sync/queue";
import { localDocs, bootstrapLocal } from "../data/store";
import { activeWs } from "../data/workspace";
import { setTasksView } from "../data/tasks-view";
import { HomePage } from "../modules/home/HomePage";
import { clearToken, getToken } from "../api/client";
import { escapeHtml } from "../ui/kit";
import { t } from "../i18n";

const WS_COLORS = ["#6366f1", "#10b981", "#f59e0b", "#ec4899", "#3b82f6", "#8b5cf6"];

// Shell: sidebar (workspaces + docs locales + búsqueda) y topbar.
// Los documentos leen del mirror local (100% offline).
export class App extends ElurComponent {
  private palette = new CommandPalette();
  private online = signal(navigator.onLine);
  private pending = signal(0);
  private authed = signal(getToken() !== null);
  private workspaces = signal<Array<{ id: string; slug: string; name: string; role: string }>>([]);
  private searchQ = signal("");
  private sidebarOpen = signal(false);
  private theme = signal<"light" | "dark" | "system">(
    (localStorage.getItem("hub:theme") as "light" | "dark" | "system") ?? "system",
  );

  onMount(): (() => void) | void {
    this.applyTheme();
    const refresh = () => {
      this.online.value = navigator.onLine;
      queueLength().then((n) => (this.pending.value = n));
    };
    window.addEventListener("online", refresh);
    window.addEventListener("offline", refresh);
    let gPressed = false;
    let gTimer: ReturnType<typeof setTimeout> | null = null;
    const onKey = (ev: KeyboardEvent) => {
      // no interceptar cuando se escribe en un input/textarea
      const tag = (ev.target as HTMLElement)?.tagName;
      const typing = tag === "INPUT" || tag === "TEXTAREA" || (ev.target as HTMLElement)?.isContentEditable;

      if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === "k") {
        ev.preventDefault();
        this.palette.toggle();
        return;
      }
      if (ev.key === "Escape") {
        this.palette.close();
        gPressed = false;
        return;
      }
      if (this.palette.isOpen() && (ev.key === "ArrowDown" || ev.key === "ArrowUp" || ev.key === "Enter")) {
        this.palette.keydown(ev);
        return;
      }
      if (typing) return;

      // atajos g + <tecla> (Linear-style)
      if (gPressed) {
        gPressed = false;
        if (gTimer) clearTimeout(gTimer);
        const routes: Record<string, string> = {
          d: "/docs", t: "/tasks", s: "/search", g: "/graph", k: "/settings",
        };
        const target = routes[ev.key.toLowerCase()];
        if (target) {
          ev.preventDefault();
          router.navigate(target);
        }
        return;
      }
      if (ev.key === "g") {
        gPressed = true;
        gTimer = setTimeout(() => (gPressed = false), 1500);
        return;
      }
      if (ev.key === "?") {
        ev.preventDefault();
        this.palette.toggle();
        return;
      }
    };
    const onOpenDoc = (ev: Event) => {
      const id = (ev as CustomEvent<string>).detail;
      if (id) router.navigate("/docs/" + id);
    };
    const onAuthChange = () => {
      this.authed.value = getToken() !== null;
      if (this.authed.value) {
        void this.bootstrap();
        if (router.current.value === "/") router.navigate("/docs");
      } else if (router.current.value !== "/") {
        router.navigate("/");
      }
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("hub:open-doc", onOpenDoc);
    window.addEventListener("hub:login", onAuthChange);
    window.addEventListener("hub:logout", onAuthChange);
    void refresh();
    void this.bootstrap();
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("hub:open-doc", onOpenDoc);
      window.removeEventListener("hub:login", onAuthChange);
      window.removeEventListener("hub:logout", onAuthChange);
      window.removeEventListener("online", refresh);
      window.removeEventListener("offline", refresh);
    };
  }

  private applyTheme(): void {
    const t = this.theme.value;
    if (t === "system") {
      document.documentElement.removeAttribute("data-theme");
    } else {
      document.documentElement.setAttribute("data-theme", t);
    }
    localStorage.setItem("hub:theme", t);
  }

  private cycleTheme(): void {
    const order: Array<"light" | "dark" | "system"> = ["light", "dark", "system"];
    const idx = order.indexOf(this.theme.value);
    this.theme.value = order[(idx + 1) % order.length];
    this.applyTheme();
  }

  private async bootstrap(): Promise<void> {
    if (!getToken()) return;
    try {
      const { workspaces } = await workspacesApi.list();
      this.workspaces.value = workspaces;
      if (workspaces.length && !activeWs.value) {
        activeWs.value = workspaces[0].id;
      }
    } catch {
      /* sin sesión */
    }
    const ws = activeWs.value;
    if (ws) await bootstrapLocal(ws);
  }

  private async switchWorkspace(id: string): Promise<void> {
    if (activeWs.value === id) return;
    activeWs.value = id;
    const ws = activeWs.value;
    if (ws) await bootstrapLocal(ws);
  }

  render(): ElurTemplate {
    // Gate: el shell permanece montado (RouterView estable) y se oculta
    // sin sesión; la pantalla auth se superpone. Evita re-mounts del
    // RouterView en transiciones de sesión.
    return html`
      <div class="app-shell" style=${() => (this.authed.value ? "" : "grid-template-columns: 1fr")}>
        <aside class=${"sidebar" + (this.sidebarOpen.value ? " open" : "")}
          style=${() => (this.authed.value ? "" : "display:none")}>
          <div class="logo">
            <div class="logo-mark"><img src="/kora-hub-logo.png" alt="Kora Hub" /></div>
            <span>Kora Hub</span>
          </div>
          <div class="sidebar-search">
            <input placeholder=${() => t("app.search_ph")}
              value=${() => this.searchQ.value}
              @input=${(ev: Event) => (this.searchQ.value = (ev.target as HTMLInputElement).value)} />
          </div>
          ${() =>
        this.authed.value
          ? html`
                  <div class="sidebar-section">
                    <div class="section-label">${() => t("app.workspaces")}</div>
                    <div class="ws-list">
                      ${this.workspaces.value.map((ws, i) => html`
                        <button class=${"ws-item" + (activeWs.value === ws.id ? " active" : "")}
                          @click=${() => void this.switchWorkspace(ws.id)}>
                          <span class="ws-dot" style=${"background:" + WS_COLORS[i % WS_COLORS.length]}></span>
                          <span class="ws-name">${ws.name}</span>
                          <span class="role-badge">${ws.role}</span>
                        </button>`)}
                    </div>
                  </div>
                  <div class="sidebar-section">
                    <div class="section-label">${() => t("app.docs")}</div>
                    <div class="docs-list">
                      ${() =>
              localDocs.value
                .filter((d) => d.workspaceId === activeWs.value)
                .filter(
                  (d) =>
                    !this.searchQ.value ||
                    d.title.toLowerCase().includes(this.searchQ.value.toLowerCase()) ||
                    d.path.toLowerCase().includes(this.searchQ.value.toLowerCase()),
                )
                .map(
                  (d) => html`
                              <button class="doc-item" @click=${() => {
                      this.sidebarOpen.value = false;
                      router.navigate("/docs/" + d.id);
                    }}>
                                <svg class="doc-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
                                <span class="doc-name">${escapeHtml(d.title)}</span>
                              </button>`,
                )}
                      ${() =>
              localDocs.value.length === 0
                ? html`<div class="muted" style="padding: 8px 12px; font-size: 12px">${() => t("app.no_docs")}</div>`
                : ""}
                    </div>
                  </div>
                  <div class="sidebar-spacer"></div>
                  <button class="new-doc-btn" @click=${() => {
              this.sidebarOpen.value = false;
              router.navigate("/docs");
            }}>
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M12 5v14M5 12h14"/></svg>
                    ${() => t("app.new_doc")}
                  </button>
                `
          : html`<div class="sidebar-spacer"></div>`}
        </aside>

        <main class="main">
          <header class="topbar" style=${() => (this.authed.value ? "" : "display:none")}>
            <button class="menu-toggle" @click=${() => (this.sidebarOpen.value = !this.sidebarOpen.value)}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="18" x2="21" y2="18"/></svg>
            </button>
            <div class="doc-breadcrumb">
              <span class="ws-tag">
                ${() => this.workspaces.value.find((w) => w.id === activeWs.value)?.name ?? "Kora Hub"}
              </span>
              <span class="sep">/</span>
              <span class="doc-name">
                ${() =>
        localDocs.value.find((d) => d.id === (router.current.value.match(/^\/docs\/(.+)$/)?.[1] ?? ""))?.title ?? ""}
              </span>
            </div>
            <div class="view-switcher">
              ${(() => {
        const views: Array<{ label: string; isActive: (cur: string) => boolean; go: () => void }> = [
          { label: t("app.nav.docs"), isActive: (c) => c.startsWith("/docs"), go: () => router.navigate("/docs") },
          { label: t("app.nav.search"), isActive: (c) => c.startsWith("/search"), go: () => router.navigate("/search") },
          { label: t("app.nav.kanban"), isActive: (c) => c.startsWith("/tasks") && !c.includes("view="), go: () => { setTasksView("kanban"); router.navigate("/tasks"); } },
          { label: t("app.nav.table"), isActive: (c) => c.includes("view=tabla"), go: () => { setTasksView("tabla"); router.navigate({ name: "tasks", query: { view: "tabla" } }); } },
          { label: t("app.nav.calendar"), isActive: (c) => c.includes("view=calendario"), go: () => { setTasksView("calendario"); router.navigate({ name: "tasks", query: { view: "calendario" } }); } },
          { label: t("app.nav.graph"), isActive: (c) => c.startsWith("/graph"), go: () => router.navigate("/graph") },
          { label: t("app.nav.settings"), isActive: (c) => c.startsWith("/settings"), go: () => router.navigate("/settings") },
        ];
        return views.map((v) => html`<button class=${"tab" + (v.isActive(router.current.value) ? " active" : "")}
                  @click=${() => v.go()}>${v.label}</button>`);
      })()}
            </div>
            <div class="sync-indicator">
              <span class=${"dot" + (this.online.value ? " dot-on" : " dot-off")}></span>
              <span class="text">${() => (this.online.value ? t("app.online") : t("app.offline"))}</span>
              ${() => (this.pending.value > 0 ? html` · ${this.pending.value} ${t("app.pending")}` : "")}
            </div>
            <button class="cmd-btn" @click=${() => this.palette.toggle()}>
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
              ${() => t("app.search")} <kbd>⌘K</kbd>
            </button>
            <button class="theme-toggle" @click=${() => this.cycleTheme()} title=${() => t("app.theme")}>
              <span class="icon">${() => this.theme.value === "dark" ? "🌙" : this.theme.value === "light" ? "☀️" : "🖥️"}</span>
            </button>
          </header>
          <main class="content">${new RouterView()}</main>
        </main>
        ${this.palette}
      </div>
    `;
  }
}
