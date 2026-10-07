// Kit de UI compartido: helpers puros para fechas, badges y
// componentes imperativos (toast, modal de prompt).

import { t, tList } from "../i18n";

export function escapeHtml(s: unknown): string {
  if (s == null) return "";
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!,
  );
}

export function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function plusISO(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function isOverdue(dateStr: string): boolean {
  if (!dateStr) return false;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return new Date(dateStr + "T00:00:00") < today;
}

export function formatDate(dateStr: string): string {
  const d = new Date(dateStr + "T00:00:00");
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const tomorrow = new Date(today);
  tomorrow.setDate(tomorrow.getDate() + 1);
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  if (d.getTime() === today.getTime()) return t("kit.today");
  if (d.getTime() === tomorrow.getTime()) return t("kit.tomorrow");
  if (d.getTime() === yesterday.getTime()) return t("kit.yesterday");

  return `${d.getDate()} ${tList("kit.months")[d.getMonth()]}`;
}

// fecha relativa -> ISO: reutiliza el parser (misma gramática Go↔TS,
// componentes locales — nunca toISOString, que da el día equivocado
// cerca de medianoche).
export { resolveDateISO } from "../tasks/parser";

// fuzzyMatch: subsecuencia de caracteres (estilo Raycast)
export function fuzzyMatch(query: string, text: string): boolean {
  if (!query) return true;
  query = query.toLowerCase();
  text = text.toLowerCase();
  let qi = 0;
  for (let i = 0; i < text.length && qi < query.length; i++) {
    if (text[i] === query[qi]) qi++;
  }
  return qi === query.length;
}

// ------------------------- Toast -------------------------

let toastEl: HTMLElement | null = null;
let toastTimer: ReturnType<typeof setTimeout> | null = null;

export function showToast(msg: string): void {
  if (!toastEl) {
    toastEl = document.createElement("div");
    toastEl.className = "toast";
    document.body.appendChild(toastEl);
  }
  toastEl.textContent = msg;
  toastEl.classList.add("show");
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toastEl?.classList.remove("show"), 2500);
}

// ------------------------- Modal prompt -------------------------

export function showPrompt(title: string, placeholder: string, defaultValue = ""): Promise<string | null> {
  return new Promise((resolve) => {
    const backdrop = document.createElement("div");
    backdrop.className = "modal-backdrop";
    backdrop.innerHTML = `
      <div class="modal">
        <h3>${escapeHtml(title)}</h3>
        <input type="text" placeholder="${escapeHtml(placeholder)}" />
        <div class="modal-actions">
          <button class="ghost" data-act="cancel">${escapeHtml(t("kit.cancel"))}</button>
          <button class="primary" data-act="confirm">${escapeHtml(t("kit.create"))}</button>
        </div>
      </div>`;
    document.body.appendChild(backdrop);

    const input = backdrop.querySelector("input")!;
    input.value = defaultValue;
    const cleanup = (result: string | null) => {
      backdrop.remove();
      resolve(result);
    };
    backdrop.querySelector('[data-act="cancel"]')!.addEventListener("click", () => cleanup(null));
    backdrop.querySelector('[data-act="confirm"]')!.addEventListener("click", () => cleanup(input.value.trim()));
    backdrop.addEventListener("click", (e) => {
      if (e.target === backdrop) cleanup(null);
    });
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") cleanup(input.value.trim());
      if (e.key === "Escape") cleanup(null);
    });
    setTimeout(() => input.focus(), 30);
  });
}

// ------------------------- Modal select -------------------------

export interface SelectOption {
  value: string;
  label: string;
  description?: string;
}

// showSelect muestra un modal con opciones clicables; devuelve el
// value elegido o null si se cancela.
export function showSelect(title: string, options: SelectOption[]): Promise<string | null> {
  return new Promise((resolve) => {
    const backdrop = document.createElement("div");
    backdrop.className = "modal-backdrop";
    backdrop.innerHTML = `
      <div class="modal">
        <h3>${escapeHtml(title)}</h3>
        <div class="select-list"></div>
        <div class="modal-actions">
          <button class="ghost" data-act="cancel">${escapeHtml(t("kit.cancel"))}</button>
        </div>
      </div>`;
    document.body.appendChild(backdrop);

    const list = backdrop.querySelector(".select-list")!;
    const cleanup = (result: string | null) => {
      backdrop.remove();
      resolve(result);
    };
    for (const opt of options) {
      const btn = document.createElement("button");
      btn.className = "select-option";
      btn.innerHTML = `<strong>${escapeHtml(opt.label)}</strong>` +
        (opt.description ? `<span class="muted">${escapeHtml(opt.description)}</span>` : "");
      btn.addEventListener("click", () => cleanup(opt.value));
      list.appendChild(btn);
    }
    backdrop.querySelector('[data-act="cancel"]')!.addEventListener("click", () => cleanup(null));
    backdrop.addEventListener("click", (e) => {
      if (e.target === backdrop) cleanup(null);
    });
    backdrop.addEventListener("keydown", (e) => {
      if (e.key === "Escape") cleanup(null);
    });
    setTimeout(() => (list.querySelector("button") as HTMLButtonElement | null)?.focus(), 30);
  });
}
