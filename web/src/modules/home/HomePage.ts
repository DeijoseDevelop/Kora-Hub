import { ElurComponent, html, signal, type ElurTemplate } from "@elurjs/core";
import { authApi, getToken, setSession } from "../../api/client";
import { router } from "../../router";
import { showToast } from "../../ui/kit";
import { t } from "../../i18n";

// Pantalla Auth: tabs "Entrar" / "Crear cuenta". En el primer arranque
// (sin usuarios registrados) "Crear cuenta" es la opción por defecto.
export class HomePage extends ElurComponent {
  private mode = signal<"login" | "register">("login");
  private email = signal("");
  private password = signal("");
  private displayName = signal("");
  private showPassword = signal(false);

  onMount(): void {
    // con sesión activa, / redirige a Documentos (la pantalla auth
    // solo es para entrar/registrarse)
    if (getToken()) {
      router.navigate("/docs");
      return;
    }
    void authApi.status().then(({ has_users }) => {
      if (!has_users) this.mode.value = "register";
    }).catch(() => undefined);
  }

  private submit(): void {
    if (this.password.value.length < 8) {
      showToast(t("auth.pw_short"));
      return;
    }
    const run = this.mode.value === "login"
      ? authApi.login(this.email.value, this.password.value)
      : authApi.register(this.email.value, this.password.value, this.displayName.value || this.email.value);
    run
      .then((session) => {
        setSession(session);
        window.dispatchEvent(new CustomEvent("hub:login"));
        showToast(this.mode.value === "register" ? t("auth.registered") : t("auth.logged_in"));
      })
      .catch((e: Error) => showToast(e.message));
  }

  render(): ElurTemplate {
    return html`
      <div class="login-view">
        <div class="login-card">
          <div class="logo-mark login-mark"><img src="/kora-hub-logo.png" alt="Kora Hub" /></div>
          <h2>Kora Hub</h2>
          <p class="login-sub">${() => t("auth.tagline")}</p>
          <div class="tabs" style="margin-bottom: 16px">
            <button class=${() => "tab" + (this.mode.value === "login" ? " active" : "")}
              @click=${() => (this.mode.value = "login")}>${() => t("auth.login")}</button>
            <button class=${() => "tab" + (this.mode.value === "register" ? " active" : "")}
              @click=${() => (this.mode.value = "register")}>${() => t("auth.register")}</button>
          </div>
          <form @submit=${(ev: Event) => {
        ev.preventDefault();
        this.submit();
      }}>
            ${() =>
        this.mode.value === "register"
          ? html`
                    <div class="field">
                      <input type="text" placeholder=${() => t("auth.name_ph")} value=${() => this.displayName.value}
                        @input=${(ev: Event) => (this.displayName.value = (ev.target as HTMLInputElement).value)} />
                    </div>`
          : ""}
            <div class="field">
              <input type="email" placeholder="Email" value=${() => this.email.value}
                @input=${(ev: Event) => (this.email.value = (ev.target as HTMLInputElement).value)} />
            </div>
            <div class="field password-field">
              <input type=${() => (this.showPassword.value ? "text" : "password")}
                placeholder=${() => t("auth.password_ph")} value=${() => this.password.value}
                @input=${(ev: Event) => (this.password.value = (ev.target as HTMLInputElement).value)} />
              <button type="button" class="eye-toggle" title=${() => t("auth.show_pw")}
                @click=${() => (this.showPassword.value = !this.showPassword.value)}>
                ${() =>
        this.showPassword.value
          ? html`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/></svg>`
          : html`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-10-8-10-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 10 8 10 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`}
              </button>
            </div>
            <button class="btn" type="submit">${() => (this.mode.value === "login" ? t("auth.login") : t("auth.register"))}</button>
          </form>
          <p class="login-hint">${() => t("auth.hint")}</p>
        </div>
      </div>
    `;
  }
}

export default HomePage;
