// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Capa de plataforma (sección 7.3): una interfaz decide la
// implementación según el entorno. Ningún módulo de negocio conoce la
// diferencia entre web/PWA y apps Capacitor.

export interface StorageAdapter {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  removeItem(key: string): Promise<void>;
}

// Web/PWA: IndexedDB vía localStorage es suficiente para settings
// ligeros en el MVP; la cola offline usa Dexie (sync/queue.ts).
export class WebStorageAdapter implements StorageAdapter {
  async getItem(key: string): Promise<string | null> {
    return localStorage.getItem(key);
  }
  async setItem(key: string, value: string): Promise<void> {
    localStorage.setItem(key, value);
  }
  async removeItem(key: string): Promise<void> {
    localStorage.removeItem(key);
  }
}

export const platformStorage: StorageAdapter = new WebStorageAdapter();
