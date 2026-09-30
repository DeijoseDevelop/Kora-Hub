import { createRouter, type RouteRecord } from "@deijose/nix-js";
import { HomePage } from "./modules/home/HomePage";
import { DocsPage } from "./modules/docs/DocsPage";
import { DocEditorPage } from "./modules/docs/DocEditorPage";
import { TasksPage } from "./modules/tasks/TasksPage";
import { SearchPage } from "./modules/search/SearchPage";
import { GraphPage } from "./modules/graph/GraphPage";
import { SettingsPage } from "./modules/settings/SettingsPage";
import { PublicDocPage } from "./modules/docs/PublicDocPage";

// Rutas con carga directa: el bundle es pequeño (<20 KB) y la navegación
// es instantánea sin estados de carga intermedios.
const routes: RouteRecord[] = [
  { path: "/", component: () => new HomePage(), meta: { auth: false } },
  { path: "/docs", component: () => new DocsPage(), meta: { auth: true } },
  { path: "/docs/:id", component: () => new DocEditorPage(), meta: { auth: true } },
  { name: "tasks", path: "/tasks", component: () => new TasksPage(), meta: { auth: true } },
  { path: "/search", component: () => SearchPage(), meta: { auth: true } },
  { path: "/graph", component: () => new GraphPage(), meta: { auth: true } },
  { path: "/settings", component: () => new SettingsPage(), meta: { auth: true } },
  // Ruta publica de share-links (D5): sin sesión, solo lectura.
  { path: "/p/:token", component: () => new PublicDocPage(), meta: { auth: false } },
];

export const router = createRouter(routes, { mode: "hash" });

// Guardia: las rutas internas requieren sesión (defensa en profundidad
// además del gate visual del shell).
import { getToken } from "./api/client";
router.beforeEach((to) => {
  const match = router.resolve(to);
  if (match?.route?.meta?.auth && !getToken()) {
    return "/";
  }
  return undefined;
});
