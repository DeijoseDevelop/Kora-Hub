import { NixComponent, html, ref, type NixTemplate } from "@deijose/nix-js";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, drawSelection, lineNumbers, highlightActiveLine } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { markdown } from "@codemirror/lang-markdown";
import { oneDark } from "@codemirror/theme-one-dark";

// Editor como NixComponent (sección 7.2): ref() al contenedor, instancia
// de CodeMirror en onMount() y cleanup automático al desmontar. Sin
// wrappers: CodeMirror es DOM-first.
//
// Nota: el contenido NO se sincroniza a una signal por keystroke (eso
// disparaba un loop de re-mount del subárbol embebido, detectado en
// E2E). Se expone onChange como callback plano para la preview.
export class MarkdownEditor extends NixComponent {
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

  render(): NixTemplate {
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
        oneDark,
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
