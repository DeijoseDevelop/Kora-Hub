// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { NixComponent, html, ref, signal, type NixTemplate } from "@deijose/nix-js";
import { marked } from "marked";
import { router } from "../../router";
import { publicDocsApi, type PublicDoc } from "../../api/client";
import { escapeHtml } from "../../ui/kit";
import { sanitizeHTML } from "../../ui/sanitize";
import { t } from "../../i18n";

// PublicDocPage es la vista pública read-only de un share-link (D5):
// sin sesión, sin mirror local, sin controles de edición. El contenido
// llega del endpoint público y se renderiza como Markdown (misma
// configuración que el preview del editor).
export class PublicDocPage extends NixComponent {
  private doc = signal<PublicDoc | null>(null);
  private error = signal("");
  private bodyRef = ref<HTMLDivElement>();

  onMount(): void {
    const token = router.params.value.token ?? "";
    if (!token) {
      this.error.value = t("public.invalid");
      return;
    }
    void this.load(token);
  }

  private async load(token: string): Promise<void> {
    try {
      const doc = await publicDocsApi.get(token);
      this.doc.value = doc;
      document.title = `${doc.title} — Kora Hub`;
      queueMicrotask(() => this.renderBody(doc.content));
    } catch {
      this.error.value = t("public.invalid");
    }
  }

  private renderBody(md: string): void {
    const el = this.bodyRef.el;
    if (!el) return;
    try {
      el.innerHTML = sanitizeHTML(marked.parse(md, { breaks: true, gfm: true }) as string);
    } catch {
      el.innerHTML = `<pre>${escapeHtml(md)}</pre>`;
    }
  }

  render(): NixTemplate {
    return html`
      <div class="page public-page">
        ${() => this.error.value
          ? html`<div class="public-error"><h2>${() => this.error.value}</h2></div>`
          : this.doc.value === null
            ? html`<p class="muted">${() => t("public.loading")}</p>`
            : html`
              <article class="public-doc">
                <header class="public-header">
                  <h1>${() => this.doc.value!.title}</h1>
                  <p class="faint">${() => formatUpdated(this.doc.value!.updated_at)}</p>
                </header>
                <div class="doc-preview public-body" ref=${this.bodyRef}></div>
                <footer class="public-footer">${() => t("public.powered")}</footer>
              </article>`}
      </div>
    `;
  }
}

function formatUpdated(iso: string): string {
  return iso.slice(0, 16).replace("T", " ");
}
