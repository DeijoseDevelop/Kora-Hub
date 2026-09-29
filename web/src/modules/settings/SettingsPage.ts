import { NixComponent, html, type NixTemplate } from "@deijose/nix-js";
import { createQuery } from "@deijose/nix-query";
import { authApi, clearToken, getToken, workspacesApi } from "../../api/client";
import { pull } from "../../sync/client";
import { showToast } from "../../ui/kit";
import { MembersPanel } from "./MembersPanel";

const ws = createQuery<
  Array<{ id: string; slug: string; name: string; role: string }> | null,
  void
>(
  "workspaces/list",
  async () => (getToken() ? (await workspacesApi.list()).workspaces : []),
  { refetchOnMount: "always" },
);

const me = createQuery<{ id: string; email: string; display_name: string } | null, void>(
  "auth/me",
  async () => (getToken() ? await authApi.me() : null),
  { refetchOnMount: "always" },
);

let name = "";
let slug = "";

function logout(): void {
  void authApi.logout().catch(() => undefined);
  clearToken();
  showToast("Sesión cerrada");
  window.dispatchEvent(new CustomEvent("hub:logout"));
}

// Página como clase: onMount refetchea (los queries module-level se
// crean antes del login y no se re-ejecutan solos — mismo patrón que
// TasksPage/DocsPage).
export class SettingsPage extends NixComponent {
  onMount(): void {
    ws.refetch();
    me.refetch();
  }

  render(): NixTemplate {
    return html`
      <section class="page">
        <div class="page-header">
          <h2>Ajustes</h2>
          <button class="btn ghost" @click=${() => logout()}>Cerrar sesión</button>
        </div>

        <h3>Cuenta</h3>
        <div class="ws-card">
          <span><strong>${() => me.data.value?.display_name ?? "…"}</strong>
            <span class="muted">${() => me.data.value?.email ?? ""}</span></span>
          <span class="role-badge">usuario</span>
        </div>

        <h3>Workspaces</h3>
        ${() =>
        (ws.data.value ?? []).map((w) => html`
            <div class="ws-card">
              <span><strong>${w.name}</strong> <span class="muted">${"@" + w.slug}</span></span>
              <span>
                <button class="btn ghost" @click=${() =>
            workspacesApi
              .exportZip(w.id, w.slug)
              .catch((e: Error) => showToast(e.message))}>
                  Exportar
                </button>
                ${w.role !== "viewer"
            ? html`<button class="btn ghost" @click=${(ev: Event) => {
              const input = (ev.target as HTMLElement).parentElement
                ?.querySelector<HTMLInputElement>("input[type=file]");
              input?.click();
            }}>Importar vault</button>
                    <input type="file" accept=".zip" style="display:none"
                      @change=${(ev: Event) => {
                const file = (ev.target as HTMLInputElement).files?.[0];
                (ev.target as HTMLInputElement).value = "";
                if (!file) return;
                workspacesApi
                  .importZip(w.id, file)
                  .then(async (r) => {
                    showToast(
                      `Importados ${r.imported} docs, ${r.attachments} adjuntos (${r.skipped} omitidos)`,
                    );
                    await pull(w.id).catch(() => undefined);
                  })
                  .catch((e: Error) => showToast(e.message));
              }} />`
            : ""}
                <span class="role-badge">${w.role}</span>
              </span>
            </div>
            ${w.role === "owner" ? new MembersPanel(w.id, w.name) : ""}
          `)}
        ${() =>
        (ws.data.value ?? []).length === 0
          ? html`<p class="muted">Cargando workspaces…</p>`
          : ""}

        <h3>Nuevo workspace</h3>
        <form class="settings-form" @submit=${(ev: Event) => {
        ev.preventDefault();
        if (!slug || !name) {
          showToast("Completa el slug y el nombre");
          return;
        }
        workspacesApi
          .create(slug, name)
          .then(() => {
            ws.refetch();
            name = "";
            slug = "";
            showToast("Workspace creado");
          })
          .catch((e: Error) => showToast(e.message));
      }}>
          <div class="field">
            <input placeholder="slug (minúsculas)" value=${() => slug}
              @input=${(ev: Event) => (slug = (ev.target as HTMLInputElement).value)} />
          </div>
          <div class="field">
            <input placeholder="nombre" value=${() => name}
              @input=${(ev: Event) => (name = (ev.target as HTMLInputElement).value)} />
          </div>
          <button class="btn" type="submit">Crear</button>
        </form>

        <p class="muted" style="margin-top: 14px">
          Roles: <strong>owner</strong> administra, <strong>editor</strong> edita,
          <strong>viewer</strong> solo lee. El backend garantiza el permiso;
          la UI solo lo refleja.
        </p>
      </section>
    `;
  }
}

export default SettingsPage;
