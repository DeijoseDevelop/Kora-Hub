// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { ElurComponent, html, ref, signal, type ElurTemplate } from "@elurjs/core";
import { localDocs, localTasks } from "../../data/store";
import { activeWs } from "../../data/workspace";
import { searchApi, getToken } from "../../api/client";
import { router } from "../../router";
import { t } from "../../i18n";

type Hit = {
  kind: "doc" | "task";
  id: string;
  title: string;
  sub: string;
  snippet: string;
};

// Búsqueda full-text local-first (sección 4.2): mirror Dexie offline +
// FTS5 del servidor cuando hay red.
//
// El input NO usa signal para el texto: en Nix.js cambiar un signal del
// componente recrea el subárbol y resetea el campo. Se lee del DOM vía
// ref y solo los resultados viven en signals.
export class SearchPage extends ElurComponent {
  private hits = signal<Hit[]>([]);
  private searching = signal(false);
  private hasQuery = signal(false);
  private tipo = signal<"todos" | "doc" | "tarea">("todos");
  private inputRef = ref<HTMLInputElement>();

  onMount(): void {
    this.inputRef.el?.focus();
  }

  private queryText(): string {
    return (this.inputRef.el?.value ?? "").trim();
  }

  private async run(): Promise<void> {
    const raw = this.queryText();
    this.hasQuery.value = raw.length > 0;
    const q = raw.toLowerCase();
    const ws = activeWs.value;
    if (!q || !ws) {
      this.hits.value = [];
      return;
    }
    const words = q.split(/\s+/).filter(Boolean);
    const match = (text: string | null) => {
      if (!text) return false;
      const low = text.toLowerCase();
      return words.every((w) => low.includes(w));
    };

    const docs = localDocs.value.filter(
      (d) => d.workspaceId === ws && !d.deleted && (match(d.title) || match(d.path) || match(d.content)),
    );
    const tasks = localTasks.value.filter(
      (tk) => tk.workspaceId === ws && (match(tk.title) || match(tk.project)),
    );

    let hits: Hit[] = [
      ...docs.slice(0, 20).map((d): Hit => ({
        kind: "doc",
        id: d.id,
        title: d.title,
        sub: d.path,
        snippet: snippet(d.content, words[0]),
      })),
      ...tasks.slice(0, 20).map((tk): Hit => ({
        kind: "task",
        id: tk.docId,
        title: tk.title,
        sub: (tk.done ? "✓ " : "") + (tk.project ? "@" + tk.project : ""),
        snippet: tk.dueDate ? "# " + tk.dueDate : "",
      })),
    ];

    // refuerzo con FTS5 del servidor cuando hay sesión y red
    if (getToken() && navigator.onLine) {
      this.searching.value = true;
      try {
        const res = await searchApi.apply(q, this.tipo.value === "todos" ? undefined : this.tipo.value === "doc" ? "doc" : "tarea");
        const remote = ((res.results ?? []) as { tipo: string; id: string; title: string; path?: string }[]).map(
          (r): Hit => ({
            kind: r.tipo === "tarea" ? "task" : "doc",
            id: r.id,
            title: r.title,
            sub: r.path ?? r.tipo,
            snippet: "",
          }),
        );
        hits.push(...remote);
      } catch {
        /* sin red o FTS roto: los locales bastan */
      } finally {
        this.searching.value = false;
      }
    }
    // dedup global: local + servidor, por kind + título normalizado
    const norm = (s: string) => s.trim().toLowerCase();
    const seen = new Set<string>();
    const deduped: Hit[] = [];
    for (const h of hits) {
      const key = h.kind + ":" + norm(h.title);
      if (!seen.has(key)) {
        seen.add(key);
        deduped.push(h);
      }
    }
    this.hits.value = deduped;
  }

  private go(hit: Hit): void {
    router.navigate("/docs/" + hit.id);
  }

  render(): ElurTemplate {
    return html`
      <div class="page">
        <div class="page-header">
          <h2>${() => t("search.title")}</h2>
          <div class="tabs">
            ${(["todos", "doc", "tarea"] as const).map(
              (v) => html`<button class=${() => "tab" + (this.tipo.value === v ? " active" : "")}
                @click=${() => { this.tipo.value = v; void this.run(); }}>${() => t({ todos: "search.all", doc: "search.docs", tarea: "search.tasks" }[v])}</button>`,
            )}
          </div>
        </div>
        <input id="search-input" class="search-box" type="text" autocomplete="off"
          ref=${this.inputRef}
          placeholder=${() => t("search.ph")}
          @input=${() => void this.run()}
          @keydown=${(ev: KeyboardEvent) => { if (ev.key === "Enter") void this.run(); }} />
        ${() => (this.searching.value ? html`<p class="muted">${() => t("search.searching")}</p>` : "")}
        <div class="search-results">
          ${() =>
            this.hits.value.length === 0
              ? html`<p class="muted">${() => (this.hasQuery.value ? t("search.empty") : t("search.desc"))}</p>`
              : this.hits.value.map(
                  (h) => html`
                    <div class="search-hit" id=${"hit-" + h.id} @click=${() => this.go(h)}>
                      <span class=${"badge " + h.kind}>${h.kind === "doc" ? "DOC" : "TASK"}</span>
                      <span class="hit-title">${h.title}</span>
                      <span class="hit-sub">${h.sub}</span>
                      ${h.snippet ? html`<span class="hit-snippet">${h.snippet}</span>` : ""}
                    </div>`,
                )}
        </div>
      </div>
    `;
  }
}

function snippet(content: string, word: string): string {
  if (!word) return "";
  const i = content.toLowerCase().indexOf(word.toLowerCase());
  if (i < 0) return "";
  const from = Math.max(0, i - 30);
  return (from > 0 ? "…" : "") + content.slice(from, i + word.length + 40).replace(/\n/g, " ");
}

export default SearchPage;
