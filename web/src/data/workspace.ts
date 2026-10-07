import { signal } from "@elurjs/core";

// Workspace activo de la UI. Las llamadas a la API incluyen
// ?workspace=<id> y el mirror local filtra por él.
export const activeWs = signal<string | null>(null);

export function workspaceParam(): string {
  return activeWs.value ? `?workspace=${encodeURIComponent(activeWs.value)}` : "";
}

// wsQuery fusiona el parámetro de workspace con una query existente.
export function wsQuery(existing: string): string {
  const ws = workspaceParam();
  if (!ws) return existing;
  return existing ? existing + "&" + ws.slice(1) : ws;
}
