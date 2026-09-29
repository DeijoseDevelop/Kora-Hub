import { NixComponent, html, type NixTemplate } from "@deijose/nix-js";
import { router } from "../../router";
import { currentRole } from "../../api/role";
import { localDocs } from "../../data/store";
import { activeWs } from "../../data/workspace";
import { createDocLocal } from "../../data/mutations";
import { showPrompt, showSelect, showToast } from "../../ui/kit";
import { t } from "../../i18n";
import { DOC_TEMPLATES, templateDesc, templateLabel } from "../../data/templates";

// Lista de documentos leída del mirror local (funciona offline).
export class DocsPage extends NixComponent {
  render(): NixTemplate {
    return html`
      <div class="page">
        <div class="page-header">
          <h2>${() => t("docs.title")}</h2>
          <button class="btn" id="new-doc" @click=${() => void this.newDoc()}>${() => t("docs.new")}</button>
        </div>
        <div class="list-table-wrap">
          <table class="list-table">
            <thead>
              <tr><th>${() => t("docs.col_title")}</th><th>${() => t("docs.col_path")}</th><th>${() => t("docs.col_updated")}</th></tr>
            </thead>
            <tbody>
              ${() =>
        localDocs.value.filter((d) => d.workspaceId === activeWs.value).map((d) => html`
                  <tr style="cursor:pointer" @click=${() => router.navigate("/docs/" + d.id)}>
                    <td><strong>${d.title}</strong></td>
                    <td><span class="faint">${d.path}</span></td>
                    <td><span class="faint">${d.updatedAt.slice(0, 10)}</span></td>
                  </tr>`)}
              ${() =>
        localDocs.value.filter((d) => d.workspaceId === activeWs.value).length === 0
          ? html`<tr><td colspan="3" class="empty-state">${() => t("docs.empty")}</td></tr>`
          : ""}
            </tbody>
          </table>
        </div>
      </div>
    `;
  }

  private async newDoc(): Promise<void> {
    const role = await currentRole();
    if (role === "viewer") {
      showToast(t("tasks.viewer_ro"));
      return;
    }
    const title = await showPrompt(t("docs.new_title"), t("docs.new_ph"));
    if (!title) return;
    // plantilla (MVP sección 9.1): se elige tras el nombre; el esqueleto
    // es contenido Markdown inicial del documento canónico.
    const tplId = await showSelect(
      t("docs.template"),
      DOC_TEMPLATES.map((tpl) => ({ value: tpl.id, label: templateLabel(tpl), description: templateDesc(tpl) })),
    );
    if (tplId == null) return;
    const tpl = DOC_TEMPLATES.find((t) => t.id === tplId) ?? DOC_TEMPLATES[0];
    const path = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") + ".md";
    try {
      const doc = await createDocLocal(title, path, tpl.skeleton(title));
      showToast(t("docs.created"));
      router.navigate("/docs/" + doc.id);
    } catch (e) {
      showToast((e as Error).message);
    }
  }
}
