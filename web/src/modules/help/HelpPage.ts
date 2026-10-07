// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { ElurComponent, html, signal, type ElurTemplate } from "@elurjs/core";
import { t } from "../../i18n";

const SECTIONS = ["features", "shortcuts", "syntax", "tasks", "tips"] as const;
type Section = (typeof SECTIONS)[number];

// HelpPage: documentación completa dentro de la herramienta — features,
// atajos, sintaxis de tareas embebidas y tips. Accesible desde el
// sidebar (botón ?) o con el atajo '?'.
export class HelpPage extends ElurComponent {
  private active = signal<Section>("features");

  render(): ElurTemplate {
    return html`
      <div class="page help-page">
        <div class="page-header">
          <h2>${() => t("help.title")}</h2>
          <div class="tabs">
            ${SECTIONS.map((s) => html`<button class=${() => "tab" + (this.active.value === s ? " active" : "")}
              @click=${() => (this.active.value = s)}>${() => t("help.tab_" + s)}</button>`)}
          </div>
        </div>
        <div class="help-content">
          ${() => this.active.value === "features" ? this.features() : ""}
          ${() => this.active.value === "shortcuts" ? this.shortcuts() : ""}
          ${() => this.active.value === "syntax" ? this.syntax() : ""}
          ${() => this.active.value === "tasks" ? this.tasks() : ""}
          ${() => this.active.value === "tips" ? this.tips() : ""}
        </div>
      </div>
    `;
  }

  private features(): ElurTemplate {
    return html`<div class="help-section">
      <h3>🚀 ${() => t("help.features_title")}</h3>
      <p class="muted">${() => t("help.features_desc")}</p>

      <div class="help-grid">
        <div class="help-card">
          <h4>📝 ${() => t("help.f_docs")}</h4>
          <p>${() => t("help.f_docs_desc")}</p>
        </div>
        <div class="help-card">
          <h4>✅ ${() => t("help.f_tasks")}</h4>
          <p>${() => t("help.f_tasks_desc")}</p>
        </div>
        <div class="help-card">
          <h4>📊 ${() => t("help.f_views")}</h4>
          <p>${() => t("help.f_views_desc")}</p>
        </div>
        <div class="help-card">
          <h4>🔄 ${() => t("help.f_offline")}</h4>
          <p>${() => t("help.f_offline_desc")}</p>
        </div>
        <div class="help-card">
          <h4>🔍 ${() => t("help.f_search")}</h4>
          <p>${() => t("help.f_search_desc")}</p>
        </div>
        <div class="help-card">
          <h4>🌙 ${() => t("help.f_theme")}</h4>
          <p>${() => t("help.f_theme_desc")}</p>
        </div>
        <div class="help-card">
          <h4>💬 ${() => t("help.f_comments")}</h4>
          <p>${() => t("help.f_comments_desc")}</p>
        </div>
        <div class="help-card">
          <h4>🔗 ${() => t("help.f_share")}</h4>
          <p>${() => t("help.f_share_desc")}</p>
        </div>
        <div class="help-card">
          <h4>📤 ${() => t("help.f_export")}</h4>
          <p>${() => t("help.f_export_desc")}</p>
        </div>
        <div class="help-card">
          <h4>📅 ${() => t("help.f_ics")}</h4>
          <p>${() => t("help.f_ics_desc")}</p>
        </div>
        <div class="help-card">
          <h4>🧮 ${() => t("help.f_math")}</h4>
          <p>${() => t("help.f_math_desc")}</p>
        </div>
        <div class="help-card">
          <h4>🌐 ${() => t("help.f_graph")}</h4>
          <p>${() => t("help.f_graph_desc")}</p>
        </div>
      </div>
    </div>`;
  }

  private shortcuts(): ElurTemplate {
    return html`<div class="help-section">
      <h3>⌨️ ${() => t("help.shortcuts_title")}</h3>
      <p class="muted">${() => t("help.shortcuts_desc")}</p>
      <table class="help-table">
        <thead><tr><th>${() => t("help.key")}</th><th>${() => t("help.action")}</th></tr></thead>
        <tbody>
          <tr><td><kbd>⌘K</kbd> / <kbd>Ctrl+K</kbd></td><td>${() => t("help.sc_palette")}</td></tr>
          <tr><td><kbd>⌘P</kbd> / <kbd>Ctrl+P</kbd></td><td>${() => t("help.sc_qs")}</td></tr>
          <tr><td><kbd>?</kbd></td><td>${() => t("help.sc_help")}</td></tr>
          <tr><td><kbd>g</kbd> <kbd>d</kbd></td><td>${() => t("help.sc_docs")}</td></tr>
          <tr><td><kbd>g</kbd> <kbd>t</kbd></td><td>${() => t("help.sc_tasks")}</td></tr>
          <tr><td><kbd>g</kbd> <kbd>s</kbd></td><td>${() => t("help.sc_search")}</td></tr>
          <tr><td><kbd>g</kbd> <kbd>g</kbd></td><td>${() => t("help.sc_graph")}</td></tr>
          <tr><td><kbd>g</kbd> <kbd>k</kbd></td><td>${() => t("help.sc_settings")}</td></tr>
          <tr><td><kbd>Esc</kbd></td><td>${() => t("help.sc_close")}</td></tr>
          <tr><td><kbd>Enter</kbd></td><td>${() => t("help.sc_confirm")}</td></tr>
          <tr><td><kbd>↑</kbd> <kbd>↓</kbd></td><td>${() => t("help.sc_navigate")}</td></tr>
        </tbody>
      </table>
    </div>`;
  }

  private syntax(): ElurTemplate {
    return html`<div class="help-section">
      <h3>📖 ${() => t("help.syntax_title")}</h3>
      <p class="muted">${() => t("help.syntax_desc")}</p>

      <h4>Markdown básico</h4>
      <pre class="help-code"># Título
## Subtítulo
**negrita** *cursiva* ~~tachado~~ \`código\`
> Cita
- Lista
1. Lista numerada
| Tabla | Tabla |
|-------|-------|
| celda | celda |
[enlace](https://ejemplo.com) ![imagen](url)</pre>

      <h4>Callouts (tipo Obsidian)</h4>
      <pre class="help-code">&gt; [!note] Nota
&gt; Contenido del callout

&gt; [!tip] Consejo
&gt; Contenido

&gt; [!warning] Advertencia
&gt; Contenido

&gt; [!danger] Peligro
&gt; Contenido

&gt; [!info] Info / [!success] / [!question] / [!bug] / [!example] / [!quote]</pre>

      <h4>Math (LaTeX)</h4>
      <pre class="help-code">Inline: $E = mc^2$
Bloque:
$$x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}$$</pre>

      <h4>Footnotes</h4>
      <pre class="help-code">Texto con referencia[^1]
[^1]: La definición de la nota.</pre>

      <h4>Slash commands</h4>
      <p class="muted">${() => t("help.slash_desc")}</p>
      <pre class="help-code">/h1 /h2 /h3 — encabezados
/bold /italic /code — formato
/list /check /numbered — listas
/quote /hr /link /image — elementos
/table /task — tabla y tarea con fecha</pre>

      <h4>Embeds</h4>
      <pre class="help-code">&lt;iframe src="https://www.youtube.com/embed/ID"&gt;&lt;/iframe&gt;
&lt;audio controls src="https://..."&gt;&lt;/audio&gt;
&lt;video controls src="https://..."&gt;&lt;/video&gt;</pre>
    </div>`;
  }

  private tasks(): ElurTemplate {
    return html`<div class="help-section">
      <h3>✅ ${() => t("help.tasks_title")}</h3>
      <p class="muted">${() => t("help.tasks_desc")}</p>

      <h4>${() => t("help.task_basic")}</h4>
      <pre class="help-code">- [ ] Tarea abierta
- [x] Tarea completada
- [~] Tarea en progreso</pre>

      <h4>${() => t("help.task_meta")}</h4>
      <pre class="help-code">- [ ] Revisar informe #2026-12-15 @ventas !alta ~deiver +urgente
         │          │        │     │      │         └ etiqueta
         │          │        │     │      └ asignado
         │          │        │     └ prioridad (baja/media/alta)
         │          │        └ proyecto
         │          └ fecha (AAAA-MM-DD | hoy | mañana | lun..dom)
         └ título</pre>

      <h4>${() => t("help.task_advanced")}</h4>
      <pre class="help-code">- [ ] Backup semanal #2026-12-01 *every:1w ^id:bak-1
- [ ] Otra tarea ^blocked-by:bak-1
- [ ] Tarea del sprint +cycle:sprint-42</pre>
      <p class="muted">${() => t("help.task_advanced_desc")}</p>

      <h4>${() => t("help.task_sub")}</h4>
      <pre class="help-code">- [ ] Tarea padre
  - [ ] Sub-tarea 1
  - [ ] Sub-tarea 2
    - [ ] Sub-sub-tarea</pre>
      <p class="muted">${() => t("help.task_sub_desc")}</p>

      <h4>${() => t("help.task_values")}</h4>
      <pre class="help-code">- [ ] Tarea @"proyecto con espacios" ~"Ana Pérez"</pre>
      <p class="muted">${() => t("help.task_values_desc")}</p>
    </div>`;
  }

  private tips(): ElurTemplate {
    return html`<div class="help-section">
      <h3>💡 ${() => t("help.tips_title")}</h3>
      <div class="help-tips">
        <div class="help-tip">
          <h4>${() => t("help.tip_offline_title")}</h4>
          <p>${() => t("help.tip_offline_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_icons_title")}</h4>
          <p>${() => t("help.tip_icons_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_wikilinks_title")}</h4>
          <p>${() => t("help.tip_wikilinks_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_group_title")}</h4>
          <p>${() => t("help.tip_group_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_bulk_title")}</h4>
          <p>${() => t("help.tip_bulk_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_print_title")}</h4>
          <p>${() => t("help.tip_print_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_mcp_title")}</h4>
          <p>${() => t("help.tip_mcp_desc")}</p>
        </div>
        <div class="help-tip">
          <h4>${() => t("help.tip_import_title")}</h4>
          <p>${() => t("help.tip_import_desc")}</p>
        </div>
      </div>
    </div>`;
  }
}

export default HelpPage;
