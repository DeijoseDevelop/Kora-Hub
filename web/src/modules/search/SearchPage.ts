// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { html, type NixTemplate } from "@deijose/nix-js";

export function SearchPage(): NixTemplate {
  return html`
    <section class="page">
      <h2>Búsqueda</h2>
      <p class="muted">
        Búsqueda instantánea en cliente con FlexSearch sobre la copia local;
        búsqueda profunda (FTS5) delegada al servidor cuando hay conexión.
      </p>
    </section>
  `;
}

export default SearchPage ;
