// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Cliente de la API REST v1 (P4: la UI es un cliente más).
// El token se guarda en localStorage; cada petición lleva Bearer.

export interface DocSummary {
  id: string;
  title: string;
  path: string;
  updated_at: string;
}

export interface DocDetail extends DocSummary {
  content: string;
  content_hash: string;
}

export interface Task {
  id: string;
  title: string;
  done: number;
  in_progress: number;
  due_date: string | null;
  project: string | null;
  priority: string | null;
  assignee: string | null;
  line_no: number;
  doc_id: string;
}

export interface SearchResult {
  id: string;
  title: string;
  path: string;
}

const TOKEN_KEY = "hub:token";
const REFRESH_KEY = "hub:refresh";
const EXPIRES_KEY = "hub:expires";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
  localStorage.removeItem(EXPIRES_KEY);
}

// setSession guarda el par de tokens + expiración del access.
export function setSession(session: { access_token: string; refresh_token: string; expires_in?: number }): void {
  localStorage.setItem(TOKEN_KEY, session.access_token);
  localStorage.setItem(REFRESH_KEY, session.refresh_token);
  if (session.expires_in) {
    localStorage.setItem(EXPIRES_KEY, String(Date.now() + session.expires_in * 1000));
  }
}

export function sessionExpired(): boolean {
  const exp = Number(localStorage.getItem(EXPIRES_KEY) ?? "0");
  return exp > 0 && Date.now() > exp;
}

let refreshInFlight: Promise<boolean> | null = null;

// refreshSession renueva el par con el refresh token (single-flight:
// las llamadas concurrentes comparten la misma petición).
export function refreshSession(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      const refresh = getRefreshToken();
      if (!refresh) return false;
      try {
        const res = await fetch("/api/v1/auth/refresh", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refresh }),
        });
        if (!res.ok) return false;
        const session = await res.json();
        setSession(session);
        return true;
      } catch {
        return false;
      } finally {
        refreshInFlight = null;
      }
    })();
  }
  return refreshInFlight;
}

// apiFetch es el helper autenticado: ante 401 renueva el token (una vez)
// y reintenta; si el refresh falla, limpia la sesión y emite hub:logout.
export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string>),
  };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;

  let res = await fetch(`/api/v1${path}`, { ...init, headers });
  if (res.status === 401) {
    const refreshed = await refreshSession();
    if (refreshed) {
      headers.Authorization = `Bearer ${getToken()}`;
      res = await fetch(`/api/v1${path}`, { ...init, headers });
    }
  }
  if (res.status === 401) {
    // tras un refresh fallido la sesión ya no existe
    clearToken();
    window.dispatchEvent(new CustomEvent("hub:logout"));
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body?.error?.message ?? `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  return apiFetch<T>(path, init);
}

export const authApi = {
  register: (email: string, password: string, displayName: string) =>
    api<{ access_token: string; refresh_token: string; expires_in?: number }>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password, display_name: displayName }),
    }),
  login: (email: string, password: string) =>
    api<{ access_token: string; refresh_token: string; expires_in?: number }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  logout: () =>
    apiFetch<void>("/auth/logout", {
      method: "POST",
      body: JSON.stringify({ refresh_token: getRefreshToken() }),
    }),
  me: () => apiFetch<{ id: string; email: string; display_name: string }>("/auth/me"),
  status: () => api<{ has_users: boolean }>("/auth/status"),
};

export interface DocVersion {
  id: number;
  content_hash: string;
  created_at: string;
  created_by: string;
}

export interface Backlink {
  id: string;
  path: string;
  title: string;
  anchor_text: string;
}

export interface Attachment {
  id: string;
  filename: string;
  mime: string;
  size_bytes: number;
  created_at: string;
}

export const docsApi = {
  list: () => api<{ docs: DocSummary[] }>("/docs"),
  get: (id: string) => api<DocDetail>(`/docs/${id}`),
  create: (path: string, title: string, content: string) =>
    api<DocSummary>("/docs", {
      method: "POST",
      body: JSON.stringify({ path, title, content }),
    }),
  update: (id: string, content: string) =>
    api<{ content_hash: string }>(`/docs/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ content }),
    }),
  versions: (id: string) => api<{ versions: DocVersion[] }>(`/docs/${id}/versions`),
  version: (id: string, vid: number) =>
    api<{ content: string; created_at: string }>(`/docs/${id}/versions/${vid}`),
  backlinks: (id: string) => api<{ backlinks: Backlink[] }>(`/docs/${id}/backlinks`),
};

export interface ShareState {
  token: string | null;
  url?: string;
  created_at?: string;
}

// Share-links públicos (D5): el token es la capacidad; regenerar rota el
// token e invalida el anterior.
export const sharesApi = {
  get: (docId: string) => api<ShareState>(`/docs/${docId}/share`),
  create: (docId: string) =>
    api<{ token: string; url: string }>(`/docs/${docId}/share`, { method: "POST" }),
  remove: (docId: string) =>
    api<void>(`/docs/${docId}/share`, { method: "DELETE" }),
};

export interface PublicDoc {
  title: string;
  path: string;
  content: string;
  updated_at: string;
}

// publicDocsApi NO usa api() — el visitante del enlace no tiene sesión.
export const publicDocsApi = {
  get: async (token: string): Promise<PublicDoc> => {
    const res = await fetch(`/api/v1/public/docs/${token}`);
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body?.error?.message ?? `HTTP ${res.status}`);
    }
    return res.json();
  },
};

export const attachmentsApi = {
  list: (workspace: string) =>
    api<{ attachments: Attachment[] }>(`/attachments?workspace=${workspace}`),
  upload: async (file: File, workspace: string, docId?: string): Promise<Attachment & { url: string }> => {
    const form = new FormData();
    form.append("file", file);
    if (docId) form.append("doc_id", docId);
    const res = await fetch(`/api/v1/attachments?workspace=${workspace}`, {
      method: "POST",
      headers: { Authorization: `Bearer ${getToken()}` },
      body: form,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body?.error?.message ?? `HTTP ${res.status}`);
    }
    return res.json();
  },
  remove: (id: string, workspace: string) =>
    api<void>(`/attachments/${id}?workspace=${workspace}`, { method: "DELETE" }),
};

export interface SavedView {
  id: string;
  name: string;
  filters: string; // JSON serializado: {vista, proyecto, done, ...}
}

export const viewsApi = {
  list: (workspace: string) =>
    api<{ views: SavedView[] }>(`/views?workspace=${workspace}`),
  create: (workspace: string, name: string, filters: Record<string, string>) =>
    api<{ id: string }>(`/views?workspace=${workspace}`, {
      method: "POST",
      body: JSON.stringify({ name, filters }),
    }),
  remove: (id: string, workspace: string) =>
    api<void>(`/views/${id}?workspace=${workspace}`, { method: "DELETE" }),
};

export const tasksApi = {
  list: (done = false) =>
    api<{ tasks: Task[] }>(`/tasks?done=${done ? 1 : 0}`),
  listVista: (vista: string, params: Record<string, string> = {}) => {
    const q = new URLSearchParams({ vista, ...params }).toString();
    return api<{ tasks: Task[] }>(`/tasks?${q}`);
  },
  patch: (id: string, patch: { done: boolean }) =>
    api<{ done: boolean }>(`/tasks/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  quickAdd: (text: string) =>
    api<Task>("/tasks", {
      method: "POST",
      body: JSON.stringify({ text }),
    }),
};

export const searchApi = {
  docs: (q: string) => api<{ results: SearchResult[] }>(`/search?q=${encodeURIComponent(q)}`),
  // tipo: doc | tarea | adjunto (sección 4.2)
  apply: (q: string, tipo?: string) =>
    api<{ results: SearchResult[] }>(
      `/search?q=${encodeURIComponent(q)}${tipo ? `&tipo=${encodeURIComponent(tipo)}` : ""}`,
    ),
};

export interface Member {
  user_id: string;
  email: string;
  display_name: string;
  role: string;
}

export const workspacesApi = {
  create: (slug: string, name: string) =>
    api<{ id: string }>("/workspaces", {
      method: "POST",
      body: JSON.stringify({ slug, name }),
    }),
  list: () =>
    api<{ workspaces: Array<{ id: string; slug: string; name: string; role: string }> }>(
      "/workspaces",
    ),
  members: (id: string) => api<{ members: Member[] }>(`/workspaces/${id}/members`),
  addMember: (id: string, email: string, role: string) =>
    api<Member>(`/workspaces/${id}/members`, {
      method: "POST",
      body: JSON.stringify({ email, role }),
    }),
  setMemberRole: (id: string, uid: string, role: string) =>
    api(`/workspaces/${id}/members/${uid}`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    }),
  removeMember: (id: string, uid: string) =>
    api<void>(`/workspaces/${id}/members/${uid}`, { method: "DELETE" }),
  // export descarga el árbol canónico del workspace como ZIP (P1).
  exportZip: async (id: string, slug: string): Promise<void> => {
    const res = await fetch(`/api/v1/workspaces/${id}/export`, {
      headers: { Authorization: `Bearer ${getToken()}` },
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const blob = await res.blob();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${slug}-export.zip`;
    a.click();
    URL.revokeObjectURL(a.href);
  },
  // importZip vuelca un vault (ZIP) al workspace; devuelve contadores.
  importZip: async (id: string, file: File): Promise<{ imported: number; attachments: number; skipped: number; indexed: number }> => {
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`/api/v1/workspaces/${id}/import`, {
      method: "POST",
      headers: { Authorization: `Bearer ${getToken()}` },
      body: form,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body?.error?.message ?? `HTTP ${res.status}`);
    }
    return res.json();
  },
};
