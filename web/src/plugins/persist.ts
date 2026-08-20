// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { persistPlugin, type NixPlugin } from "@deijose/nix-js";
import { platformStorage } from "../platform/storage";

// Plugin de persistencia de stores (sección 7.2): debounce de 300 ms,
// respaldado por la capa PlatformStorage (web vs nativa).
export function persist<T extends Record<string, unknown>>(key: string): NixPlugin<T> {
  return persistPlugin(key, {
    storage: platformStorage,
    debounce: 300,
  });
}
