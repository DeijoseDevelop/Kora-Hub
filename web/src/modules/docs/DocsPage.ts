import { NixComponent, html, type NixTemplate } from "@deijose/nix-js";
import { router } from "../../router";
import { currentRole } from "../../api/role";
import { localDocs } from "../../data/store";
import { activeWs } from "../../data/workspace";
import { createDocLocal } from "../../data/mutations";
import { showPrompt, showSelect, showToast } from "../../ui/kit";
import { DOC_TEMPLATES } from "../../data/templates";

// Lista de documentos leída del mirror local (funciona offline).
export class DocsPage extends NixComponent {
  render(): NixTemplate {
    return html`
      <div class="page">
        <div class="page-header">
          <h2>Documentos</h2>
          <button class="btn" id="new-doc" @click=${() => void this.newDoc()}>Nuevo documento</button>
        </div>
        <div class="list-table-wrap">
          <table class="list-table">
            <thead>
              <tr><th>Título</th><th>Ruta</th><th>Actualizado</th></tr>
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
          ? html`<tr><td colspan="3" class="empty-state">Sin documentos. Crea uno nuevo.</td></tr>`
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
      showToast("rol viewer: solo lectura");
      return;
    }
    const title = await showPrompt("Nuevo documento", "Nombre del documento");
    if (!title) return;
    // plantilla (MVP sección 9.1): se elige tras el nombre; el esqueleto
    // es contenido Markdown inicial del documento canónico.
    const tplId = await showSelect(
      "Plantilla",
      DOC_TEMPLATES.map((t) => ({ value: t.id, label: t.label, description: t.description })),
    );
    if (tplId == null) return;
    const tpl = DOC_TEMPLATES.find((t) => t.id === tplId) ?? DOC_TEMPLATES[0];
    const path = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") + ".md";
    try {
      const doc = await createDocLocal(title, path, tpl.skeleton(title));
      showToast("Documento creado");
      router.navigate("/docs/" + doc.id);
    } catch (e) {
      showToast((e as Error).message);
    }
  }
}
