import { signal } from "@elurjs/core";
import { getLocalDocs, getLocalTasks, reindexAll, type LocalDoc, type LocalTask } from "../sync/local";
import { pull } from "../sync/client";
import { activeWs } from "./workspace";

// Fuente de lectura de la UI (local-first): los signals se alimentan
// del mirror local; el servidor solo sincroniza. 100% offline.
export const localDocs = signal<LocalDoc[]>([]);
export const localTasks = signal<LocalTask[]>([]);


export async function refreshLocal(workspaceId?: string): Promise<void> {
  localDocs.value = await getLocalDocs(workspaceId);
  localTasks.value = await getLocalTasks(workspaceId);
}

// bootstrapLocal: trae el delta del workspace, reindexa y puebla la UI.
export async function bootstrapLocal(workspaceId: string): Promise<void> {
  try {
    await pull(workspaceId);
  } catch {
    /* offline: el mirror local manda */
  }
  await reindexAll();
  await refreshLocal(workspaceId);
}

// initLocal: reacciona a cambios locales (crear/editar docs) con un
// pull + reindex + refresh en cadena del workspace activo.
export function initLocal(): void {
  window.addEventListener("hub:docs-changed", () => {
    void (async () => {
      const ws = activeWs.value;
      if (!ws) return;
      try {
        await pull(ws);
      } catch {
        /* sin conexión */
      }
      await reindexAll();
      await refreshLocal(ws);
    })();
  });
}
