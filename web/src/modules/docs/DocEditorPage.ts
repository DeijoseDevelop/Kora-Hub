import { ElurComponent, html, ref, signal, type ElurTemplate } from "@elurjs/core";
import { marked } from "marked";
import { router } from "../../router";
import { getLocalDocById, saveDocLocal } from "../../data/mutations";
import { MarkdownEditor } from "./MarkdownEditor";
import { parseLine } from "../../tasks/parser";
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
