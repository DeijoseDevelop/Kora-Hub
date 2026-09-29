// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
import { html, type NixTemplate } from "@deijose/nix-js";
import { t } from "../../i18n";

export function SearchPage(): NixTemplate {
  return html`
    <section class="page">
      <h2>${() => t("search.title")}</h2>
      <p class="muted">${() => t("search.desc")}</p>
    </section>
  `;
}

export default SearchPage;
