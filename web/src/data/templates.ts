// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Plantillas de documento (MVP sección 9.1): propuesta, acta de
// reunión, RFC y retrospectiva. Son contenido inicial Markdown —
// cliente-side para funcionar offline y sin endpoint dedicado.

export interface DocTemplate {
  id: string;
  label: string;
  description: string;
  skeleton: (title: string) => string;
}

const blank: DocTemplate = {
  id: "blank",
  label: "Documento vacío",
  description: "Empieza desde cero",
  skeleton: (t) => `# ${t}\n\n`,
};

export const DOC_TEMPLATES: DocTemplate[] = [
  blank,
  {
    id: "propuesta",
    label: "Propuesta",
    description: "Propuesta de funcionalidad o cambio",
    skeleton: (t) => `# ${t}

## Contexto


## Propuesta


## Alternativas consideradas


## Tareas

- [ ]  #hoy @proyecto

## Decisiones

`,
  },
  {
    id: "acta",
    label: "Acta de reunión",
    description: "Minuta con acuerdos y acciones",
    skeleton: (t) => `# ${t}

**Fecha:** ${new Date().toISOString().slice(0, 10)}
**Asistentes:**

## Temas tratados


## Acuerdos


## Acciones

- [ ]  ~responsable #fecha

`,
  },
  {
    id: "rfc",
    label: "RFC",
    description: "Request for comments técnico",
    skeleton: (t) => `# RFC: ${t}

**Estado:** borrador
**Autor:**

## Resumen


## Motivación


## Diseño


## Plan de implementación

- [ ]  #hoy

## Preguntas abiertas

`,
  },
  {
    id: "retro",
    label: "Retrospectiva",
    description: "Retro de sprint o proyecto",
    skeleton: (t) => `# ${t}

**Periodo:**

## Qué salió bien


## Qué mejorar


## Acciones

- [ ]  ~responsable

`,
  },
];