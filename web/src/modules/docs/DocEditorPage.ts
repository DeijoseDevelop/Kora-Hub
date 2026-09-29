import { NixComponent, html, ref, signal, type NixTemplate } from "@deijose/nix-js";
import { marked } from "marked";
import { router } from "../../router";
import { getLocalDocById, saveDocLocal } from "../../data/mutations";
import { MarkdownEditor } from "./MarkdownEditor";
import { parseLine } from "../../tasks/parser";
import { escapeHtml, formatDate, isOverdue, showToast } from "../../ui/kit";
import { attachmentsApi, docsApi, type Backlink, type DocVersion } from "../../api/client";
import { activeWs } from "../../data/workspace";
import { lineDiff } from "./diff";

// Vista de edición local-first: el documento se lee del mirror (100%
// offline); el guardado escribe el mirror, reindexa y encola el sync.
export class DocEditorPage extends NixComponent {
  private editor = new MarkdownEditor("", (text) => this.updatePreview(text), (f) => this.uploadAttachment(f));
  private status = signal("");
  private current: { id: string; path: string; title: string } | null = null;
  private previewRef = ref<HTMLDivElement>();
  private versions = signal<DocVersion[] | null>(null);
  private diffOf = signal<{ vid: number; created: string } | null>(null);
  private diffRef = ref<HTMLElement>();
  private backlinks = signal<Backlink[] | null>(null);

  onMount(): void {
    const id = router.params.value.id ?? "";
    if (!id) {
      this.status.value = "error: sin id de documento";
      return;
    }
    void this.load(id);
  }

  private async load(id: string): Promise<void> {
    const doc = await getLocalDocById(id);
    if (!doc) {
      this.status.value = "no encontrado";
      return;
    }
    this.current = { id: doc.id, path: doc.path, title: doc.title };
    this.editor.setDoc(doc.content);
    this.updatePreview(doc.content);
  }

  render(): NixTemplate {
    return html`
      <div class="page">
        <div class="page-header">
          <div class="doc-breadcrumb">
            <span class="doc-name">${() => this.current?.title ?? "Documento"}</span>
            <span class=${() => "save-indicator" + (this.status.value ? " visible" : "")}>${() => this.status.value || "Guardado"}</span>
          </div>
          <div class="doc-actions">
            <button class="btn ghost" id="toggle-versions" @click=${() => void this.toggleVersions()}>Historial</button>
            <button class="btn" @click=${() => this.save()}>Guardar</button>
          </div>
        </div>
        <div class="doc-split">
          ${this.editor}
          <div class="doc-preview" ref=${this.previewRef}></div>
        </div>
        ${() => this.diffOf.value ? html`
          <div class="diff-panel">
            <div class="diff-header">
              <span>Versión del ${this.diffOf.value!.created.slice(0, 16)} → actual</span>
              <button class="btn ghost sm" @click=${() => (this.diffOf.value = null)}>Cerrar</button>
            </div>
            <pre class="diff-body" ref=${this.diffRef}></pre>
          </div>
        ` : ""}
        ${() => this.versions.value !== null ? html`
          <div class="versions-panel">
            <h4>Historial de versiones</h4>
            ${this.versions.value!.length === 0
          ? html`<p class="muted">Sin versiones aún — se crean al guardar cambios.</p>`
          : this.versions.value!.map((v) => html`
                  <div class="version-row" @click=${() => void this.showDiff(v)}>
                    <span>${v.created_at.slice(0, 16).replace("T", " ")}</span>
                    <span class="faint mono">${v.content_hash.slice(0, 8)}</span>
                  </div>`)}
          </div>
        ` : ""}
        ${() => this.backlinks.value !== null && this.backlinks.value!.length > 0 ? html`
          <div class="backlinks-panel">
            <h4>Backlinks</h4>
            ${this.backlinks.value!.map((b) => html`
              <div class="backlink-row" @click=${() => router.navigate("/docs/" + b.id)}>
                <strong>${b.title}</strong> <span class="faint">${b.path}</span>
              </div>`)}
          </div>
        ` : ""}
      </div>
    `;
  }

  // uploadAttachment sube el archivo al backend (requiere conexión; el
  // sync offline de binarios está pendiente — §10) e inserta el enlace.
  private async uploadAttachment(file: File): Promise<string | null> {
    const ws = activeWs.value;
    if (!ws) {
      showToast("Sin workspace activo");
      return null;
    }
    try {
      const att = await attachmentsApi.upload(file, ws, this.current?.id);
      showToast(`Adjunto subido: ${att.filename}`);
      return att.url;
    } catch (e) {
      showToast((e as Error).message);
      return null;
    }
  }

  private async toggleVersions(): Promise<void> {
    if (this.versions.value !== null) {
      this.versions.value = null;
      this.diffOf.value = null;
      return;
    }
    if (!this.current) return;
    try {
      const res = await docsApi.versions(this.current.id);
      this.versions.value = res.versions;
      const bl = await docsApi.backlinks(this.current.id);
      this.backlinks.value = bl.backlinks;
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  // showDiff muestra el diff línea a línea entre el snapshot y el
  // contenido actual del editor.
  private async showDiff(v: DocVersion): Promise<void> {
    if (!this.current) return;
    try {
      const res = await docsApi.version(this.current.id, v.id);
      const diff = lineDiff(res.content, this.editor.getDoc());
      this.diffOf.value = { vid: v.id, created: v.created_at };
      // el ref se monta tras el render del signal; deferir el innerHTML
      queueMicrotask(() => {
        const el = this.diffRef.el;
        if (!el) return;
        el.innerHTML = diff
          .map((l) => {
            const cls = l.op === "add" ? "diff-add" : l.op === "del" ? "diff-del" : "";
            const sign = l.op === "add" ? "+" : l.op === "del" ? "-" : " ";
            return `<span class="${cls}">${sign} ${escapeHtml(l.text)}</span>`;
          })
          .join("\n");
      });
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private updatePreview(md: string): void {
    const preview = this.previewRef.el;
    if (!preview) return;
    let html: string;
    try {
      html = marked.parse(md, { breaks: true, gfm: true }) as string;
    } catch {
      html = `<pre>${escapeHtml(md)}</pre>`;
    }
    preview.innerHTML = html;
    this.enhanceTaskLines(preview);
  }

  private enhanceTaskLines(preview: HTMLElement): void {
    const items = Array.from(preview.querySelectorAll("li"));
    for (const li of items) {
      // GFM convierte '- [ ]' / '- [x]' en <input type="checkbox">
      // (el texto del li ya no empieza por '['); '- [~]' queda literal.
      const box = li.querySelector('input[type="checkbox"]') as HTMLInputElement | null;
      const text = li.textContent ?? "";
      if (!box && !/^\[[ xX~]\]\s/.test(text)) continue;

      const state = box ? (box.checked ? "x" : " ") : text[1];
      const clone = li.cloneNode(true) as HTMLElement;
      clone.querySelectorAll("input").forEach((n) => n.remove());
      let rest = (clone.textContent ?? "").trim();
      // '- [~]' lo deja marked como literal: quitar el prefijo '[~]'
      if (!box) rest = rest.replace(/^\[[ xX~]\]\s*/, "");

      const task = parseLine(`- [${state}] ${rest}`);
      if (!task) continue;

      const badges: string[] = [];
      if (task.dueDate) {
        badges.push(`<span class="badge date ${isOverdue(task.dueDate) ? "overdue" : ""}">${formatDate(task.dueDate)}</span>`);
      }
      if (task.project) badges.push(`<span class="badge project">@${escapeHtml(task.project)}</span>`);
      if (task.priority) badges.push(`<span class="badge priority-${escapeHtml(task.priority)}">${escapeHtml(task.priority)}</span>`);
      if (task.assignee) badges.push(`<span class="badge assignee">~${escapeHtml(task.assignee)}</span>`);
      for (const tg of task.tags) badges.push(`<span class="badge tag">+${escapeHtml(tg)}</span>`);

      const div = document.createElement("div");
      div.className = `task-line ${task.done ? "done" : task.inProgress ? "doing" : ""}`;
      div.innerHTML = `
        <div class="task-checkbox ${task.done ? "checked" : task.inProgress ? "in-progress" : ""}"></div>
        <div class="task-body">
          <span class="task-text">${escapeHtml(task.title)}</span>
          <span class="task-badges">${badges.join("")}</span>
        </div>`;
      li.replaceWith(div);
    }
  }

  private save(): void {
    const doc = this.current;
    if (!doc) return;
    this.status.value = "guardando…";
    saveDocLocal({ id: doc.id, path: doc.path, title: doc.title, content: this.editor.getDoc() })
      .then(() => (this.status.value = "Guardado"))
      .catch(() => {
        this.status.value = "error";
        showToast("No se pudo guardar");
      });
  }
}
