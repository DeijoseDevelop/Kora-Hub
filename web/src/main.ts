import { mount } from "@elurjs/core";
import { App } from "./app/App";
import { initSyncAuto } from "./sync/client";
import { initLocal } from "./data/store";
import "@fontsource/inter/400.css";
import "@fontsource/inter/500.css";
import "@fontsource/inter/600.css";
import "@fontsource/inter/700.css";
import "@fontsource/inter/800.css";
import "@fontsource/poppins/600.css";
import "@fontsource/poppins/700.css";
import "@fontsource/jetbrains-mono/400.css";
import "./style.css";

mount(new App(), "#app");

// Offline-first (P2): el mirror local es la fuente de lectura; el sync
// es opcional y converge.
initLocal();
initSyncAuto();
