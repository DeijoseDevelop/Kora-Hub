// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import type { CapacitorConfig } from "@capacitor/cli";

// ADR-05: Capacitor directo, sin Ionic. Un solo dist/ alimenta web,
// PWA, iOS y Android (sección 8.2).
const config: CapacitorConfig = {
  appId: "dev.kora.hub",
  appName: "Kora Hub",
  webDir: "dist",
  server: {
    androidScheme: "https",
  },
};

export default config;
