import { ElurComponent, html, signal, type ElurTemplate } from "@elurjs/core";
import { createQuery } from "@elurjs/query";
import { activityApi, authApi, clearToken, getToken, workspacesApi, type ActivityEntry } from "../../api/client";
import { pull } from "../../sync/client";
import { showToast } from "../../ui/kit";
import { MembersPanel } from "./MembersPanel";
import { locale, setLocale, t, type Locale } from "../../i18n";

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
  showToast(t("settings.logout_done"));
  window.dispatchEvent(new CustomEvent("hub:logout"));
}

// Página como clase: onMount refetchea (los queries module-level se
// crean antes del login y no se re-ejecutan solos — mismo patrón que
// TasksPage/DocsPage).
export class SettingsPage extends ElurComponent {
  private activity = signal<ActivityEntry[] | null>(null);

  onMount(): void {
    ws.refetch();
    me.refetch();
    this.loadActivity();
  }

  private async loadActivity(): Promise<void> {
    const wsId = ws.data.value?.[0]?.id;
    if (!wsId) return;
    try {
      const res = await activityApi.list(wsId);
      this.activity.value = res.activity.slice(0, 30);
    } catch {
      this.activity.value = [];
    }
  }

  render(): ElurTemplate {
    return html`
      <section class="page">
        <div class="page-header">
          <h2>${() => t("settings.title")}</h2>
          <button class="btn ghost" @click=${() => logout()}>${() => t("settings.logout")}</button>
        </div>

        <h3>${() => t("settings.account")}</h3>
        <div class="ws-card">
          <span><strong>${() => me.data.value?.display_name ?? "…"}</strong>
            <span class="muted">${() => me.data.value?.email ?? ""}</span></span>
          <span class="role-badge">${() => t("settings.user")}</span>
        </div>

        <h3>${() => t("settings.language")}</h3>
        <div class="ws-card">
          <select value=${() => locale.value}
            @change=${(ev: Event) => setLocale((ev.target as HTMLSelectElement).value as Locale)}>
            <option value="es">Español</option>
            <option value="en">English</option>
          </select>
        </div>

        <h3>${() => t("settings.workspaces")}</h3>
        ${() =>
        (ws.data.value ?? []).map((w) => html`
            <div class="ws-card">
              <span><strong>${w.name}</strong> <span class="muted">${"@" + w.slug}</span></span>
              <span>
                <button class="btn ghost" @click=${() =>
            workspacesApi
              .exportZip(w.id, w.slug)
              .catch((e: Error) => showToast(e.message))}>
                  ${() => t("settings.export")}
                </button>
                ${w.role !== "viewer"
            ? html`<button class="btn ghost" @click=${(ev: Event) => {
              const input = (ev.target as HTMLElement).parentElement
                ?.querySelector<HTMLInputElement>("input[type=file]");
              input?.click();
            }}>${() => t("settings.import")}</button>
                    <input type="file" accept=".zip" style="display:none"
                      @change=${(ev: Event) => {
                const file = (ev.target as HTMLInputElement).files?.[0];
                (ev.target as HTMLInputElement).value = "";
                if (!file) return;
                workspacesApi
                  .importZip(w.id, file)
                  .then(async (r) => {
                    showToast(
                      t("settings.imported", { docs: r.imported, atts: r.attachments, skipped: r.skipped }),
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
          ? html`<p class="muted">${() => t("settings.loading_ws")}</p>`
          : ""}

        <h3>${() => t("activity.title")}</h3>
        <div class="ws-card" style="flex-direction:column;align-items:stretch">
          ${() => (this.activity.value ?? []).length === 0
          ? html`<p class="muted">${() => t("activity.empty")}</p>`
          : html`<div class="activity-list">
              ${(this.activity.value ?? []).map((a) => html`
                <div class="activity-row">
                  <span class=${"activity-op " + a.op}>${a.op === "upsert" ? t("activity.doc_updated") : a.op === "delete" ? t("activity.doc_deleted") : a.op === "create" ? t("activity.doc_created") : t("activity.comment")}</span>
                  <span class="activity-doc">${a.doc_title || a.doc_path}</span>
                  <span class="activity-date">${a.created_at.slice(0, 16).replace("T", " ")}</span>
                </div>`)}
            </div>`}
        </div>

        <h3>${() => t("settings.new_ws")}</h3>
        <form class="settings-form" @submit=${(ev: Event) => {
        ev.preventDefault();
        if (!slug || !name) {
          showToast(t("settings.fill_fields"));
          return;
        }
        workspacesApi
          .create(slug, name)
          .then(() => {
            ws.refetch();
            name = "";
            slug = "";
            showToast(t("settings.ws_created"));
          })
          .catch((e: Error) => showToast(e.message));
      }}>
          <div class="field">
            <input placeholder=${() => t("settings.slug_ph")} value=${() => slug}
              @input=${(ev: Event) => (slug = (ev.target as HTMLInputElement).value)} />
          </div>
          <div class="field">
            <input placeholder=${() => t("settings.name_ph")} value=${() => name}
              @input=${(ev: Event) => (name = (ev.target as HTMLInputElement).value)} />
          </div>
          <button class="btn" type="submit">${() => t("settings.create")}</button>
        </form>

        <p class="muted" style="margin-top: 14px">${() => t("settings.roles_hint")}</p>
      </section>
    `;
  }
}

export default SettingsPage;
