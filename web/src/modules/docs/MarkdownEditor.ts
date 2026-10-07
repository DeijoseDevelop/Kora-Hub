import { ElurComponent, html, ref, type ElurTemplate } from "@elurjs/core";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, drawSelection, lineNumbers, highlightActiveLine } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { autocompletion, completeFromList, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { markdown } from "@codemirror/lang-markdown";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";

// Slash commands: '/' al inicio de línea o tras espacio abre un menú
// de inserción Markdown (estilo Notion).
const SLASH_COMMANDS = [
  { label: "/h1", detail: "Encabezado 1", apply: "# " },
  { label: "/h2", detail: "Encabezado 2", apply: "## " },
  { label: "/h3", detail: "Encabezado 3", apply: "### " },
  { label: "/bold", detail: "Negrita", apply: "****" },
  { label: "/italic", detail: "Cursiva", apply: "**" },
  { label: "/code", detail: "Código inline", apply: "``" },
  { label: "/codeblock", detail: "Bloque de código", apply: "```\n\n```" },
  { label: "/list", detail: "Lista", apply: "- " },
  { label: "/check", detail: "Tarea", apply: "- [ ] " },
  { label: "/numbered", detail: "Lista numerada", apply: "1. " },
  { label: "/quote", detail: "Cita", apply: "> " },
  { label: "/hr", detail: "Separador", apply: "---\n" },
  { label: "/link", detail: "Enlace", apply: "[](url)" },
  { label: "/image", detail: "Imagen", apply: "![](url)" },
  { label: "/table", detail: "Tabla", apply: "| Columna | Columna |\n|---------|----------|\n| celda   | celda    |" },
  { label: "/task", detail: "Tarea con fecha", apply: "- [ ] Nueva tarea #hoy" },
];

function slashComplete(ctx: CompletionContext): CompletionResult | null {
  const word = ctx.matchBefore(/\/[a-z]*/);
  if (!word || (word.from === word.to && !ctx.explicit)) return null;
  return {
    from: word.from,
    options: SLASH_COMMANDS.map((c) => ({
      label: c.label,
      detail: c.detail,
      apply: c.apply,
      type: "keyword",
    })),
    validFor: /^\/[a-z]*$/,
  };
}

// Theme claro del design system v3: superficies neutras, un solo acento
// cian, tipografía mono para el cuerpo. Sustituye a oneDark (oscuro)
// que era inconsistente con el resto de la UI clara.
const koraLight = EditorView.theme({
  "&": {
    backgroundColor: "var(--bg)",
    color: "var(--text)",
    height: "100%",
  },
  ".cm-scroller": {
    fontFamily: "var(--font-mono)",
    lineHeight: "1.75",
  },
  ".cm-content": {
    caretColor: "var(--accent)",
    padding: "16px 0",
  },
  ".cm-cursor, .cm-dropCursor": {
    borderLeftColor: "var(--accent)",
    borderLeftWidth: "2px",
  },
  "&.cm-focused .cm-cursor": {
    borderLeftColor: "var(--accent)",
  },
  ".cm-selectionBackground, ::selection": {
    backgroundColor: "var(--accent-soft) !important",
  },
  "&.cm-focused .cm-selectionBackground": {
    backgroundColor: "var(--accent-soft) !important",
  },
  ".cm-activeLine": {
    backgroundColor: "var(--bg-soft)",
  },
  ".cm-gutters": {
    backgroundColor: "var(--bg)",
    color: "var(--text-faint)",
    borderRight: "1px solid var(--border-soft)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "var(--bg-soft)",
    color: "var(--text-dim)",
  },
  ".cm-foldPlaceholder": {
    backgroundColor: "var(--bg-soft)",
    border: "1px solid var(--border)",
    color: "var(--text-dim)",
  },
});

const koraHighlight = HighlightStyle.define([
  { tag: t.heading1, color: "var(--n-800)", fontWeight: "700", fontSize: "1.35em" },
  { tag: t.heading2, color: "var(--n-800)", fontWeight: "600", fontSize: "1.15em" },
  { tag: t.heading3, color: "var(--n-700)", fontWeight: "600" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "700" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: t.link, color: "var(--accent)", textDecoration: "underline", textUnderlineOffset: "2px" },
  { tag: t.url, color: "var(--accent-hover)" },
  { tag: t.monospace, color: "var(--accent-hover)", fontFamily: "var(--font-mono)", fontSize: "0.92em" },
  { tag: t.quote, color: "var(--text-dim)", fontStyle: "italic" },
  { tag: t.list, color: "var(--text)" },
  { tag: t.atom, color: "var(--accent)" },
  { tag: t.meta, color: "var(--text-faint)" },
  { tag: t.processingInstruction, color: "var(--text-faint)" },
]);

// Editor como ElurComponent (sección 7.2): ref() al contenedor, instancia
// de CodeMirror en onMount() y cleanup automático al desmontar. Sin
// wrappers: CodeMirror es DOM-first.
//
// Nota: el contenido NO se sincroniza a una signal por keystroke (eso
// disparaba un loop de re-mount del subárbol embebido, detectado en
// E2E). Se expone onChange como callback plano para la preview.
export class MarkdownEditor extends ElurComponent {
  private container = ref<HTMLDivElement>();
  private view: EditorView | null = null;
  private initial: string;
  private onChange?: (text: string) => void;
  private onFile?: (file: File) => Promise<string | null>;

  constructor(initial: string, onChange?: (text: string) => void, onFile?: (file: File) => Promise<string | null>) {
    super();
    this.initial = initial;
    this.onChange = onChange;
    this.onFile = onFile;
  }

  render(): ElurTemplate {
    return html`<div class="doc-editor" ref=${this.container}></div>`;
  }

  onMount(): (() => void) | void {
    if (!this.container.el) return;
    const state = EditorState.create({
      doc: this.initial,
      extensions: [
        lineNumbers(),
        highlightActiveLine(),
        drawSelection(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
        markdown(),
        autocompletion({
          override: [slashComplete],
          icons: false,
          activateOnTyping: true,
        }),
        koraLight,
        syntaxHighlighting(koraHighlight),
        EditorView.lineWrapping,
        EditorView.updateListener.of((u) => {
          if (u.docChanged) {
            this.onChange?.(u.state.doc.toString());
          }
        }),
        // drag & drop / pegar adjuntos (sección 9.1): se suben al
        // backend y se inserta el enlace Markdown en la posición del
        // drop o del cursor
        EditorView.domEventHandlers({
          drop: (ev, view) => {
            const file = ev.dataTransfer?.files?.[0];
            if (!file || !this.onFile) return;
            ev.preventDefault();
            const pos = view.posAtCoords({ x: ev.clientX, y: ev.clientY }) ?? view.state.selection.main.head;
            void this.insertAt(view, file, pos);
          },
          paste: (ev, view) => {
            const file = ev.clipboardData?.files?.[0];
            if (!file || !this.onFile) return;
            ev.preventDefault();
            void this.insertAt(view, file, view.state.selection.main.head);
          },
        }),
      ],
    });
    this.view = new EditorView({ state, parent: this.container.el });
    return () => this.view?.destroy();
  }

  // insertAt sube el archivo e inserta el enlace Markdown devuelto
  // (el handler devuelve la URL absoluta de la API del adjunto).
  private async insertAt(view: EditorView, file: File, pos: number): Promise<void> {
    const url = await this.onFile?.(file);
    if (!url) return;
    const isImage = file.type.startsWith("image/");
    const link = (isImage ? `![${file.name}](` : `[${file.name}](`) + url + ")";
    view.dispatch({ changes: { from: pos, insert: link } });
  }

  setDoc(content: string): void {
    if (this.view) {
      this.view.dispatch({ changes: { from: 0, to: this.view.state.doc.length, insert: content } });
    } else {
      this.initial = content;
    }
  }

  getDoc(): string {
    return this.view ? this.view.state.doc.toString() : this.initial;
  }
}
