// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { createStore } from "@elurjs/core";
import { persist } from "../plugins/persist";

export interface Workspace {
  id: string;
  slug: string;
  name: string;
  role: "owner" | "editor" | "viewer";
}

// Estado por dominio con createStore (sección 7.2): cada propiedad se
// auto-signaliza; el plugin de persistencia vuelca cambios a la capa
// local con debounce de 300 ms y los hidrata al arrancar.
export const workspaces = createStore(
  {
    current: null as Workspace | null,
    list: [] as Workspace[],
    ready: false,
  },
  {
    name: "workspaces",
    actions: (s) => ({
      select(id: string) {
        s.current.value = s.list.value.find((w) => w.id === id) ?? null;
      },
      hydrate(list: Workspace[]) {
        s.list.value = list;
        s.ready.value = true;
      },
    }),
    plugins: [persist("hub:workspaces")],
  },
);
