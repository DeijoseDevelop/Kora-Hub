// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { ElurComponent, html, signal, type ElurTemplate } from "@elurjs/core";
import { workspacesApi, type Member } from "../../api/client";
import { showToast } from "../../ui/kit";
import { t } from "../../i18n";

// Panel de miembros de un workspace (solo owners lo ven): lista,
// invitación por email y cambio de rol/expulsión. El backend es la
// autoridad — la UI solo refleja (sección 4.3).
export class MembersPanel extends ElurComponent {
  private wsId: string;
  private wsName: string;
  private members = signal<Member[]>([]);
  private email = "";
  private role = "editor";

  constructor(wsId: string, wsName: string) {
    super();
    this.wsId = wsId;
    this.wsName = wsName;
  }

  onMount(): void {
    void this.load();
  }

  private async load(): Promise<void> {
    try {
      const res = await workspacesApi.members(this.wsId);
      this.members.value = res.members;
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private async invite(): Promise<void> {
    if (!this.email.trim()) return;
    try {
      await workspacesApi.addMember(this.wsId, this.email.trim(), this.role);
      this.email = "";
      showToast(t("settings.member_added"));
      await this.load();
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private async setRole(uid: string, role: string): Promise<void> {
    try {
      await workspacesApi.setMemberRole(this.wsId, uid, role);
      await this.load();
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  private async remove(uid: string): Promise<void> {
    try {
      await workspacesApi.removeMember(this.wsId, uid);
      showToast(t("settings.member_removed"));
      await this.load();
    } catch (e) {
      showToast((e as Error).message);
    }
  }

  render(): ElurTemplate {
    return html`
      <div class="members-panel">
        ${() =>
        this.members.value.map((m) => html`
            <div class="member-row">
              <span><strong>${m.display_name}</strong> <span class="muted">${m.email}</span></span>
              <span class="member-actions">
                <select value=${m.role} @change=${(ev: Event) =>
            void this.setRole(m.user_id, (ev.target as HTMLSelectElement).value)}>
                  <option value="owner">owner</option>
                  <option value="editor">editor</option>
                  <option value="viewer">viewer</option>
                </select>
                <button class="btn ghost sm" @click=${() => void this.remove(m.user_id)}>✕</button>
              </span>
            </div>
          `)}
        <form class="member-invite" @submit=${(ev: Event) => {
        ev.preventDefault();
        void this.invite();
      }}>
          <input type="email" placeholder=${() => t("settings.members_email_ph")} required
            @input=${(ev: Event) => (this.email = (ev.target as HTMLInputElement).value)} />
          <select @change=${(ev: Event) => (this.role = (ev.target as HTMLSelectElement).value)}>
            <option value="editor">editor</option>
            <option value="viewer">viewer</option>
            <option value="owner">owner</option>
          </select>
          <button class="btn sm" type="submit">${() => t("settings.invite")}</button>
        </form>
        <p class="muted sm">${() => t("settings.members_hint")}</p>
      </div>
    `;
  }
}