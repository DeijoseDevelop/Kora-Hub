import { ElurComponent, html, ref, signal, type ElurTemplate } from "@elurjs/core";
import { marked } from "marked";
import { router } from "../../router";
import { getLocalDocById, saveDocLocal } from "../../data/mutations";
import { MarkdownEditor } from "./MarkdownEditor";
import { parse, parseLine } from "../../tasks/parser";
import { escapeHtml, formatDate, isOverdue, showToast } from "../../ui/kit";
import { sanitizeHTML } from "../../ui/sanitize";
import { attachmentsApi, commentsApi, docsApi, sharesApi, type Backlink, type Comment, type DocVersion, type ShareState } from "../../api/client";
import { activeWs } from "../../data/workspace";
import { lineDiff } from "./diff";
import { t } from "../../i18n";

// Vista de edición local-first: el documento se lee del mirror (100%
// offline); el guardado escribe el mirror, reindexa y encola el sync.
export class DocEditorPage extends ElurComponent {
  private editor = new MarkdownEditor("", (text) => this.updatePreview(text), (f) => this.uploadAttachment(f));
  private status = signal("");
  private current: { id: string; path: string; title: string; serverId?: string } | null = null;
  private previewRef = ref<HTMLDivElement>();
  private versions = signal<DocVersion[] | null>(null);
  private diffOf = signal<{ vid: number; created: string } | null>(null);
  private diffRef = ref<HTMLElement>();
  private backlinks = signal<Backlink[] | null>(null);
  // undefined = panel cerrado; null-token = doc sin share activo.
  private share = signal<ShareState | undefined>(undefined);
  private comments = signal<Comment[] | null>(null);
  private commentText = "";
  private toc = signal<{ id: string; text: string; level: number }[] | null>(null);

  onMount(): void {
    const id = router.params.value.id ?? "";
    if (!id) {
      this.status.value = t("editor.no_id");
      return;
    }
    void this.load(id);
  }

  private async load(id: string): Promise<void> {
    const doc = await getLocalDocById(id);
    if (!doc) {
      this.status.value = t("editor.not_found");
      return;
    }
    this.current = { id: doc.id, path: doc.path, title: doc.title, serverId: doc.serverId };
    this.editor.setDoc(doc.content);
    this.updatePreview(doc.content);
  }

  render(): ElurTemplate {
    return html`
      <div class="page">
        <div class="page-header">
          <div class="doc-breadcrumb">
            <span class="doc-name">${() => this.current?.title ?? t("editor.doc")}</span>
            <span class=${() => "save-indicator" + (this.status.value ? " visible" : "")}>${() => this.status.value || t("editor.saved")}</span>
          </div>
          <div class="doc-actions">
            <button class="btn ghost" id="toggle-versions" @click=${() => void this.toggleVersions()}>${() => t("editor.history")}</button>
            <button class="btn ghost" id="toggle-comments" @click=${() => void this.toggleComments()}>${() => t("editor.comments")} <span class="comment-count">${() => this.comments.value?.length ?? 0}</span></button>
            <button class="btn ghost" id="toggle-share" @click=${() => void this.toggleShare()}>${() => t("editor.share")}</button>
            <button class="btn ghost" id="toggle-toc" @click=${() => void this.toggleToc()}>${() => t("editor.toc")}</button>
            <button class="btn ghost" id="print-doc" @click=${() => window.print()}>${() => t("editor.print")}</button>
            <button class="btn ghost" id="export-html" @click=${() => this.exportHTML()}>${() => t("editor.export_html")}</button>
            <button class="btn ghost" id="export-ics" @click=${() => this.exportICS()}>${() => t("editor.export_ics")}</button>
            <button class="btn" @click=${() => this.save()}>${() => t("editor.save")}</button>
          </div>
        </div>
        <div class="doc-split">
          ${this.editor}
          <div class="doc-preview" ref=${this.previewRef}></div>
        </div>
        ${() => this.share.value !== undefined ? html`
          <div class="share-panel">
            <p class="muted">${() => t("share.note")}</p>
            ${this.share.value?.token ? html`
              <div class="share-url-row">
                <input class="share-url" readonly value=${() => this.shareUrl()} />
                <button class="btn sm" @click=${() => void this.copyShare()}>${() => t("share.copy")}</button>
                <button class="btn ghost sm" @click=${() => void this.createShare()}>${() => t("share.regenerate")}</button>
                <button class="btn ghost sm" @click=${() => void this.revokeShare()}>${() => t("share.revoke")}</button>
              </div>
            ` : html`
              <div class="share-url-row">
                <span class="muted">${() => t("share.none")}</span>
                <button class="btn sm" @click=${() => void this.createShare()}>${() => t("share.create")}</button>
              </div>`}
          </div>
        ` : ""}
        ${() => this.diffOf.value ? html`
          <div class="diff-panel">
            <div class="diff-header">
              <span>${() => t("editor.version_diff", { date: this.diffOf.value!.created.slice(0, 16) })}</span>
              <button class="btn ghost sm" @click=${() => (this.diffOf.value = null)}>${() => t("editor.close")}</button>
            </div>
            <pre class="diff-body" ref=${this.diffRef}></pre>
          </div>
        ` : ""}
        ${() => this.versions.value !== null ? html`
          <div class="versions-panel">
            <h4>${() => t("editor.versions")}</h4>
            ${this.versions.value!.length === 0
          ? html`<p class="muted">${() => t("editor.no_versions")}</p>`
          : this.versions.value!.map((v) => html`
                  <div class="version-row" @click=${() => void this.showDiff(v)}>
                    <span>${v.created_at.slice(0, 16).replace("T", " ")}</span>
                    <span class="faint mono">${v.content_hash.slice(0, 8)}</span>
                  </div>`)}
          </div>
        ` : ""}
        ${() => this.comments.value !== null ? html`
          <div class="comments-panel">
            <h4>${() => t("editor.comments")}</h4>
            ${(this.comments.value ?? []).length === 0
          ? html`<p class="muted">${() => t("editor.no_comments")}</p>`
          : (this.comments.value ?? []).map((cm) => html`
                  <div class="comment-row">
                    <span class="comment-author">${cm.author}</span>
                    <span class="comment-date faint">${cm.created_at.slice(0, 16).replace("T", " ")}</span>
                    <p class="comment-text">${cm.text}</p>
                  </div>`)}
            <div class="comment-input-row">
              <input id="comment-input" class="comment-input" placeholder=${() => t("editor.comment_ph")}
                value=${() => this.commentText}
                @input=${(ev: Event) => (this.commentText = (ev.target as HTMLInputElement).value)}
                @keydown=${(ev: KeyboardEvent) => { if (ev.key === "Enter") void this.addComment(); }} />
              <button class="btn sm" @click=${() => void this.addComment()}>${() => t("editor.comment_send")}</button>
            </div>
          </div>
        ` : ""}
        ${() => this.toc.value !== null && (this.toc.value ?? []).length > 0 ? html`
          <div class="toc-panel">
            <h4>${() => t("editor.toc")}</h4>
            ${(this.toc.value ?? []).map((h) => html`
              <a class=${"toc-item toc-h" + h.level} href=${"#" + h.id}
                @click=${(ev: Event) => {
                  ev.preventDefault();
                  document.getElementById(h.id)?.scrollIntoView({ behavior: "smooth" });
                }}>${h.text}</a>`)}
          </div>
        ` : ""}
        ${() => this.backlinks.value !== null && this.backlinks.value!.length > 0 ? html`
          <div class="backlinks-panel">
            <h4>${() => t("editor.backlinks")}</h4>
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
      showToast(t("editor.no_ws"));
      return null;
    }
    try {
      const att = await attachmentsApi.upload(file, ws, this.current?.id);
      showToast(t("editor.uploaded", { name: att.filename }));
      return att.url;
    } catch (e) {
      showToast((e as Error).message);
      return null;
    }
  }

  // exportHTML descarga el preview como archivo HTML autocontenido.
  private exportHTML(): void {
    const preview = this.previewRef.el;
    if (!preview) return;
    const title = this.current?.title ?? "documento";
    const html = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(title)} — Kora Hub</title>
<style>
body { font-family: Inter, system-ui, sans-serif; max-width: 720px; margin: 2rem auto; padding: 0 1rem; color: #1b2a4a; line-height: 1.7; }
h1, h2, h3 { font-family: Poppins, Inter, sans-serif; letter-spacing: -0.02em; }
h1 { font-size: 24px; } h2 { font-size: 19px; } h3 { font-size: 15px; }
code { background: #f0f2f5; padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
pre { background: #f0f2f5; padding: 14px; border-radius: 10px; overflow-x: auto; border: 1px solid #e3e9f0; }
pre code { background: none; padding: 0; }
blockquote { border-left: 3px solid #00b4d8; padding-left: 14px; color: #46597a; margin: 12px 0; }
.badge { font-size: 10.5px; padding: 2px 7px; border-radius: 99px; font-weight: 500; display: inline-block; }
.badge.date { background: #e0f2fe; color: #0284c7; }
.badge.project { background: #f0f2f5; color: #46597a; }
.badge.priority-alta { background: #fee2e2; color: #dc2626; }
.badge.priority-media { background: #fef3c7; color: #d97706; }
.badge.priority-baja { background: #dcfce7; color: #16a34a; }
.task-line { display: flex; gap: 10px; padding: 8px 10px; border-radius: 6px; background: #f8f9fa; margin: 6px 0; border: 1px solid #e3e9f0; }
.task-line.done { opacity: 0.55; text-decoration: line-through; }
.task-checkbox { width: 16px; height: 16px; border: 1.5px solid #8496b3; border-radius: 4px; flex-shrink: 0; margin-top: 2px; }
.task-checkbox.checked { background: #00b4d8; border-color: #00b4d8; }
.task-body { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.task-badges { display: inline-flex; gap: 4px; flex-wrap: wrap; }
.callout { margin: 12px 0; padding: 12px 16px; border-radius: 10px; border-left: 3px solid #00b4d8; background: #f0f2f5; }
.callout-header { font-weight: 600; font-size: 13px; margin-bottom: 4px; }
.callout-body { font-size: 13px; color: #46597a; }
.callout-note { border-left-color: #3b82f6; background: #eff6ff; }
.callout-tip { border-left-color: #18a058; background: #f0fdf4; }
.callout-warning { border-left-color: #d97706; background: #fffbeb; }
.callout-danger { border-left-color: #e5484d; background: #fef2f2; }
img { max-width: 100%; border-radius: 8px; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #e3e9f0; padding: 8px 12px; text-align: left; }
th { background: #f8f9fa; }
</style>
</head>
<body>
${preview.innerHTML}
</body>
</html>`;
    const blob = new Blob([html], { type: "text/html" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${title.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}.html`;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  // exportICS genera un archivo .ics con las tareas que tienen fecha.
  private exportICS(): void {
    const doc = this.current;
    if (!doc) return;
    const content = this.editor.getDoc();
    const tasks = parse(content).filter((t) => t.dueDate);
    if (tasks.length === 0) {
      showToast(t("editor.no_dated_tasks"));
      return;
    }
    const now = new Date().toISOString().replace(/[-:]/g, "").split(".")[0] + "Z";
    let ics = `BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Kora Hub//ES\r\n`;
    for (const task of tasks) {
      const d = task.dueDate!.replace(/-/g, "");
      ics += `BEGIN:VEVENT\r\n`;
      ics += `UID:${crypto.randomUUID()}@kora-hub\r\n`;
      ics += `DTSTAMP:${now}\r\n`;
      ics += `DTSTART;VALUE=DATE:${d}\r\n`;
      ics += `SUMMARY:${task.title.replace(/\n/g, " ")}\r\n`;
      if (task.project) ics += `CATEGORIES:${task.project}\r\n`;
      if (task.assignee) ics += `ATTENDEE:${task.assignee}\r\n`;
      ics += `DESCRIPTION:${(task.priority ? "[!" + task.priority + "] " : "")}${task.title.replace(/\n/g, " ")}\r\n`;
      ics += `END:VEVENT\r\n`;
    }
    ics += `END:VCALENDAR\r\n`;
    const blob = new Blob([ics], { type: "text/calendar" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${doc.title.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}.ics`;
    a.click();
    URL.revokeObjectURL(a.href);
    showToast(t("editor.ics_exported", { count: tasks.length }));
  }

  private toggleToc(): void {
    if (this.toc.value !== null) {
      this.toc.value = null;
      return;
    }
    const preview = this.previewRef.el;
    if (!preview) return;
    const headings = Array.from(preview.querySelectorAll("h1, h2, h3, h4"));
    headings.forEach((h, i) => {
      if (!h.id) h.id = `toc-${i}`;
    });
    this.toc.value = headings.map((h) => ({
      id: h.id,
      text: h.textContent ?? "",
      level: parseInt(h.tagName[1]),
    }));
  }

  private async toggleComments(): Promise<void> {
    if (this.comments.value !== null) {
      this.comments.value = null;
      return;
    }
    const serverId = this.current?.serverId ?? this.current?.id;
    if (!serverId || serverId.startsWith("local-")) {
      showToast(t("editor.sync_first"));
      return;
    }
    try {
      const res = await commentsApi.list(serverId);
      this.comments.value = res.comments;
    } catch {
      this.comments.value = [];
    }
  }

  private async addComment(): Promise<void> {
    const text = this.commentText.trim();
    if (!text) return;
    const serverId = this.current?.serverId ?? this.current?.id;
    if (!serverId || serverId.startsWith("local-")) {
      showToast(t("editor.sync_first"));
      return;
    }
    try {
      await commentsApi.add(serverId, text);
      this.commentText = "";
      const res = await commentsApi.list(serverId);
      this.comments.value = res.comments;
    } catch (e) {
      showToast((e as Error).message);
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
      const res = await docsApi.versions(this.serverDocId());
      this.versions.value = res.versions;
      const bl = await docsApi.backlinks(this.serverDocId());
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
      const res = await docsApi.version(this.serverDocId(), v.id);
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

  // serverDocId resuelve el id del servidor: los docs creados offline
  // llevan id "local-*" hasta el primer sync — los endpoints REST por id
  // solo entienden el id del servidor (ver sync/local.ts applyChanges).
  private serverDocId(): string {
    return this.current?.serverId ?? this.current?.id ?? "";
  }

  // Share-link público (D5): crear/regenerar/revocar son editor+ — el
  // backend es la autoridad; la UI solo refleja el resultado.
  private async toggleShare(): Promise<void> {
    if (this.share.value !== undefined) {
      this.share.value = undefined;
      return;
    }
    if (!this.current) return;
    // Relee el mirror: serverId aparece tras el primer sync; sin él el
    // backend aún no conoce el doc y compartir es imposible.
    const fresh = await getLocalDocById(this.current.id);
    if (fresh?.serverId) this.current.serverId = fresh.serverId;
    if (!this.current.serverId) {
      showToast(t("share.sync_pending"));
      return;
    }
    try {
      this.share.value = await sharesApi.get(this.serverDocId());
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private async createShare(): Promise<void> {
    if (!this.current) return;
    try {
      const res = await sharesApi.create(this.serverDocId());
      this.share.value = { token: res.token, url: res.url };
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private async revokeShare(): Promise<void> {
    if (!this.current) return;
    try {
      await sharesApi.remove(this.serverDocId());
      this.share.value = { token: null };
      showToast(t("share.revoked"));
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private shareUrl(): string {
    const token = this.share.value?.token;
    return token ? `${location.origin}${location.pathname}#/p/${token}` : "";
  }

  private async copyShare(): Promise<void> {
    try {
      await navigator.clipboard.writeText(this.shareUrl());
      showToast(t("share.copied"));
    } catch {
      showToast(t("share.copy_failed"));
    }
  }

  private updatePreview(md: string): void {
    const preview = this.previewRef.el;
    if (!preview) return;
    let html: string;
    try {
      html = sanitizeHTML(marked.parse(md, { breaks: true, gfm: true }) as string);
    } catch {
      html = `<pre>${escapeHtml(md)}</pre>`;
    }
    preview.innerHTML = html;
    this.enhanceTaskLines(preview);
    this.enhanceCallouts(preview);
    this.enhanceMath(preview);
    this.enhanceFootnotes(preview);
  }

  // enhanceMath renderiza $...$ (inline) y $$...$$ (block) como spans
  // estilizados de LaTeX — sin dependencias externas (font monospace +
  // estilo visual). KaTeX completo queda para v2.
  private enhanceMath(preview: HTMLElement): void {
    const walk = (node: Node) => {
      if (node.nodeType === Node.TEXT_NODE) {
        const text = node.textContent ?? "";
        if (!text.includes("$")) return;
        const frag = document.createDocumentFragment();
        // $$block$$ primero, luego $inline$
        const re = /(\$\$[\s\S]+?\$\$)|(\$[^$\n]+?\$)/g;
        let last = 0;
        let m: RegExpExecArray | null;
        while ((m = re.exec(text)) !== null) {
          if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
          const isBlock = !!m[1];
          const code = (m[1] ?? m[2]).replace(/^\$+|\$+$/g, "");
          const span = document.createElement(isBlock ? "div" : "span");
          span.className = isBlock ? "math-block" : "math-inline";
          span.textContent = code;
          frag.appendChild(span);
          last = m.index + m[0].length;
        }
        if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
        node.parentNode?.replaceChild(frag, node);
        return;
      }
      if (node.nodeType === Node.ELEMENT_NODE) {
        // no entrar en code/pre
        const tag = (node as Element).tagName;
        if (tag === "CODE" || tag === "PRE" || tag === "SCRIPT" || tag === "STYLE") return;
        for (const child of Array.from(node.childNodes)) walk(child);
      }
    };
    walk(preview);
  }

  // enhanceFootnotes renderiza [^1] como superscript y colecta las
  // definiciones al final del documento.
  private enhanceFootnotes(preview: HTMLElement): void {
    const defs = new Map<string, string>();
    // recolectar definiciones: líneas que empiezan por [^id]:
    const allText = preview.textContent ?? "";
    const defRe = /^\[\^(\w+)\]:\s*(.+)$/gm;
    let d: RegExpExecArray | null;
    while ((d = defRe.exec(allText)) !== null) {
      defs.set(d[1], d[2].trim());
    }
    // reemplazar referencias [^id] por superscript
    const walk = (node: Node) => {
      if (node.nodeType === Node.TEXT_NODE) {
        const text = node.textContent ?? "";
        if (!text.includes("[^")) return;
        const frag = document.createDocumentFragment();
        const re = /\[\^(\w+)\]/g;
        let last = 0;
        let m: RegExpExecArray | null;
        while ((m = re.exec(text)) !== null) {
          if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
          const id = m[1];
          const sup = document.createElement("sup");
          sup.className = "fn-ref";
          sup.textContent = id;
          sup.title = defs.get(id) ?? "";
          frag.appendChild(sup);
          last = m.index + m[0].length;
        }
        if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
        node.parentNode?.replaceChild(frag, node);
        return;
      }
      if (node.nodeType === Node.ELEMENT_NODE) {
        const tag = (node as Element).tagName;
        if (tag === "CODE" || tag === "PRE") return;
        for (const child of Array.from(node.childNodes)) walk(child);
      }
    };
    walk(preview);
    // sección de notas al final
    if (defs.size > 0) {
      const section = document.createElement("div");
      section.className = "fn-section";
      section.innerHTML = `<hr><h4>Notas</h4>` + [...defs.entries()]
        .map(([id, text]) => `<p class="fn-note"><sup>${id}</sup> ${text}</p>`)
        .join("");
      preview.appendChild(section);
    }
  }

  // enhanceCallouts convierte blockquotes '> [!type] Título' en callouts
  // estilizados (Obsidian-style: note, tip, warning, danger, info).
  private enhanceCallouts(preview: HTMLElement): void {
    const quotes = Array.from(preview.querySelectorAll("blockquote"));
    for (const bq of quotes) {
      const text = bq.textContent ?? "";
      const m = /^\[!(\w+)\]\s*(.*)/.exec(text.trim());
      if (!m) continue;
      const type = m[1].toLowerCase();
      const title = m[2].trim() || type.charAt(0).toUpperCase() + type.slice(1);
      const icons: Record<string, string> = {
        note: "📝", tip: "💡", warning: "⚠️", danger: "🚨", info: "ℹ️",
        example: "📋", quote: "💬", question: "❓", success: "✅", bug: "🐛",
      };
      const icon = icons[type] ?? "📝";
      const body = bq.innerHTML.replace(/^[^<]*<\/?p>/g, "").replace(/^\[!\w+\]\s*<p>/, "<p>").trim();
      const callout = document.createElement("div");
      callout.className = `callout callout-${type}`;
      callout.innerHTML = `
        <div class="callout-header">
          <span class="callout-icon">${icon}</span>
          <span class="callout-title">${escapeHtml(title)}</span>
        </div>
        <div class="callout-body">${body || escapeHtml(text.replace(/^\[!\w+\]\s*/, ""))}</div>`;
      bq.replaceWith(callout);
    }
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
    this.status.value = t("editor.saving");
    saveDocLocal({ id: doc.id, path: doc.path, title: doc.title, content: this.editor.getDoc() })
      .then(() => (this.status.value = t("editor.saved")))
      .catch(() => {
        this.status.value = t("editor.error");
        showToast(t("editor.save_failed"));
      });
  }
}
