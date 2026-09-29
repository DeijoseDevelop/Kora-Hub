// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Plantillas de documento (MVP sección 9.1): propuesta, acta de
// reunión, RFC y retrospectiva. Son contenido inicial Markdown —
// cliente-side para funcionar offline y sin endpoint dedicado.
// Las etiquetas y los esqueletos siguen el idioma activo (D4).

import { t } from "../i18n";

export interface DocTemplate {
  id: string;
  labelKey: string;
  descKey: string;
  skeleton: (title: string) => string;
}

const blank: DocTemplate = {
  id: "blank",
  labelKey: "tpl.blank",
  descKey: "tpl.blank_desc",
  skeleton: (title) => `# ${title}\n\n`,
};

export const DOC_TEMPLATES: DocTemplate[] = [
  blank,
  {
    id: "propuesta",
    labelKey: "tpl.propuesta",
    descKey: "tpl.propuesta_desc",
    skeleton: (title) => `# ${title}

## ${t("tpl.sec.contexto")}


## ${t("tpl.sec.propuesta")}


## ${t("tpl.sec.alternativas")}


## ${t("tpl.sec.tareas")}

- [ ]  #hoy @proyecto

## ${t("tpl.sec.decisiones")}

`,
  },
  {
    id: "acta",
    labelKey: "tpl.acta",
    descKey: "tpl.acta_desc",
    skeleton: (title) => `# ${title}

**${t("tpl.fecha")}:** ${new Date().toISOString().slice(0, 10)}
**${t("tpl.asistentes")}:**

## ${t("tpl.sec.temas")}


## ${t("tpl.sec.acuerdos")}


## ${t("tpl.sec.acciones")}

- [ ]  ~responsable #fecha

`,
  },
  {
    id: "rfc",
    labelKey: "tpl.rfc",
    descKey: "tpl.rfc_desc",
    skeleton: (title) => `# RFC: ${title}

**${t("tpl.estado")}:** ${t("tpl.borrador")}
**${t("tpl.autor")}:**

## ${t("tpl.sec.resumen")}


## ${t("tpl.sec.motivacion")}


## ${t("tpl.sec.diseno")}


## ${t("tpl.sec.plan")}

- [ ]  #hoy

## ${t("tpl.sec.preguntas")}

`,
  },
  {
    id: "retro",
    labelKey: "tpl.retro",
    descKey: "tpl.retro_desc",
    skeleton: (title) => `# ${title}

**${t("tpl.periodo")}:**

## ${t("tpl.sec.bien")}


## ${t("tpl.sec.mejorar")}


## ${t("tpl.sec.acciones")}

- [ ]  ~responsable

`,
  },
];

// label/desc resueltas en el idioma activo al renderizar el selector.
export function templateLabel(tpl: DocTemplate): string {
  return t(tpl.labelKey);
}

export function templateDesc(tpl: DocTemplate): string {
  return t(tpl.descKey);
}
