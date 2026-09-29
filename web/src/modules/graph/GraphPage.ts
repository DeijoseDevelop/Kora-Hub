import { NixComponent, effect, html, ref, type NixTemplate } from "@deijose/nix-js";
import { router } from "../../router";
import { localDocs } from "../../data/store";
import { activeWs } from "../../data/workspace";
import { extractBacklinks } from "../../sync/local";
import { t } from "../../i18n";

interface GNode {
  id: string;
  title: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  dragging: boolean;
}

interface GEdge {
  from: string;
  to: string;
}

// Grafo local-first: nodos = docs del mirror, aristas = backlinks
// [[wikilinks]] extraídos localmente. 100% offline.
export class GraphPage extends NixComponent {
  private canvasRef = ref<HTMLCanvasElement>();
  private nodes: GNode[] = [];
  private edges: GEdge[] = [];
  private anim: number | null = null;
  private dispose: (() => void) | null = null;
  private rect = { width: 0, height: 0 };
  private dragging: GNode | null = null;

  render(): NixTemplate {
    return html`
      <div class="view-graph">
        <canvas class="graph-canvas" ref=${this.canvasRef}></canvas>
        <div class="graph-legend">
          <div class="legend-item"><span class="legend-dot"></span>${() => t("graph.doc")}</div>
          <div class="legend-item"><span class="legend-line"></span>Backlink [[]]</div>
          <div class="graph-hint">${() => t("graph.hint")}</div>
        </div>
      </div>
    `;
  }

  onMount(): (() => void) | void {
    // Los refs se resuelven tras el primer render: bind y build en el
    // frame siguiente para que measure() vea el tamaño real.
    requestAnimationFrame(() => {
      this.bindCanvas();
      this.build();
    });
    // Re-construir cuando el mirror local cambie (los docs llegan por
    // pull async tras montar: sin esto el grafo queda vacío).
    this.dispose = effect(() => {
      localDocs.value;
      this.build();
    });
    window.addEventListener("resize", this.onResize);
    return () => {
      if (this.dispose) this.dispose();
      window.removeEventListener("resize", this.onResize);
      if (this.anim) cancelAnimationFrame(this.anim);
    };
  }

  private onResize = () => {
    this.measure();
    if (this.nodes.length) this.animate();
  };

  private measure(): void {
    const c = this.canvasRef.el;
    if (!c) return;
    const dpr = window.devicePixelRatio || 1;
    this.rect = { width: c.clientWidth, height: c.clientHeight };
    c.width = Math.max(1, c.clientWidth * dpr);
    c.height = Math.max(1, c.clientHeight * dpr);
  }

  private build(): void {
    const docs = localDocs.value.filter((d) => d.workspaceId === activeWs.value);
    if (!docs.length) return;
    this.measure();
    const { width, height } = this.rect;
    const existing = new Map(this.nodes.map((n) => [n.id, n]));
    this.nodes = docs.map((d, i) => {
      const prev = existing.get(d.id);
      if (prev) {
        prev.title = d.title;
        return prev;
      }
      const angle = (i / Math.max(1, docs.length)) * Math.PI * 2;
      const r = Math.min(width, height) * 0.28;
      return {
        id: d.id,
        title: d.title,
        x: width / 2 + Math.cos(angle) * r,
        y: height / 2 + Math.sin(angle) * r,
        vx: 0,
        vy: 0,
        dragging: false,
      };
    });

    const byTitle = new Map(docs.map((d) => [d.title.toLowerCase(), d.id]));
    const seen = new Set<string>();
    this.edges = [];
    for (const d of docs) {
      for (const link of extractBacklinks(d.content)) {
        const target = byTitle.get(link.dstTitle.toLowerCase());
        if (!target || target === d.id) continue;
        const key = d.id + "::" + target;
        if (seen.has(key)) continue;
        seen.add(key);
        this.edges.push({ from: d.id, to: target });
      }
    }
    this.bindCanvas();
    this.animate();
  }

  private bindCanvas(): void {
    const c = this.canvasRef.el;
    if (!c) return;
    const down = (e: MouseEvent) => {
      const x = e.clientX - c.getBoundingClientRect().left;
      const y = e.clientY - c.getBoundingClientRect().top;
      const node = this.nodes.find((n) => (n.x - x) ** 2 + (n.y - y) ** 2 < 400);
      if (node) {
        this.dragging = node;
        node.dragging = true;
      }
    };
    const move = (e: MouseEvent) => {
      if (!this.dragging) return;
      const r = c.getBoundingClientRect();
      this.dragging.x = e.clientX - r.left;
      this.dragging.y = e.clientY - r.top;
      this.dragging.vx = 0;
      this.dragging.vy = 0;
    };
    const up = () => {
      if (this.dragging) {
        this.dragging.dragging = false;
        this.dragging = null;
      }
    };
    const dbl = (e: MouseEvent) => {
      const r = c.getBoundingClientRect();
      const x = e.clientX - r.left;
      const y = e.clientY - r.top;
      const node = this.nodes.find((n) => (n.x - x) ** 2 + (n.y - y) ** 2 < 400);
      if (node) router.navigate("/docs/" + node.id);
    };
    c.onmousedown = down;
    c.onmousemove = move;
    c.onmouseup = up;
    c.onmouseleave = up;
    c.ondblclick = dbl;
  }

  private animate(): void {
    if (this.anim) cancelAnimationFrame(this.anim);
    const c = this.canvasRef.el;
    if (!c) return;
    const ctx = c.getContext("2d");
    if (!ctx) return;
    const dpr = window.devicePixelRatio || 1;
    const { width, height } = this.rect;
    const nodes = this.nodes;
    const edges = this.edges;

    const simulate = () => {
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          const a = nodes[i];
          const b = nodes[j];
          const dx = b.x - a.x;
          const dy = b.y - a.y;
          const distSq = Math.max(100, dx * dx + dy * dy);
          const dist = Math.sqrt(distSq);
          const force = 2500 / distSq;
          const fx = (dx / dist) * force;
          const fy = (dy / dist) * force;
          if (!a.dragging) { a.vx -= fx; a.vy -= fy; }
          if (!b.dragging) { b.vx += fx; b.vy += fy; }
        }
      }
      for (const e of edges) {
        const a = nodes.find((n) => n.id === e.from);
        const b = nodes.find((n) => n.id === e.to);
        if (!a || !b) continue;
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        const dist = Math.max(1, Math.sqrt(dx * dx + dy * dy));
        const force = (dist - 160) * 0.025;
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;
        if (!a.dragging) { a.vx += fx; a.vy += fy; }
        if (!b.dragging) { b.vx -= fx; b.vy -= fy; }
      }
      for (const n of nodes) {
        if (n.dragging) continue;
        n.vx += (width / 2 - n.x) * 0.001;
        n.vy += (height / 2 - n.y) * 0.001;
        n.vx *= 0.85;
        n.vy *= 0.85;
        n.x += n.vx;
        n.y += n.vy;
        n.x = Math.max(50, Math.min(width - 50, n.x));
        n.y = Math.max(50, Math.min(height - 50, n.y));
      }
    };

    const draw = () => {
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.scale(dpr, dpr);
      ctx.clearRect(0, 0, width, height);
      for (const e of edges) {
        const a = nodes.find((n) => n.id === e.from);
        const b = nodes.find((n) => n.id === e.to);
        if (!a || !b) continue;
        ctx.strokeStyle = "rgba(99, 102, 241, 0.35)";
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.stroke();
      }
      for (const n of nodes) {
        const grad = ctx.createRadialGradient(n.x, n.y, 0, n.x, n.y, 32);
        grad.addColorStop(0, "rgba(99, 102, 241, 0.18)");
        grad.addColorStop(1, "rgba(99, 102, 241, 0)");
        ctx.fillStyle = grad;
        ctx.beginPath();
        ctx.arc(n.x, n.y, 32, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = "#161922";
        ctx.strokeStyle = "#6366f1";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(n.x, n.y, 8, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
        ctx.fillStyle = "#e6e8ee";
        ctx.font = "500 12px Inter, sans-serif";
        ctx.textAlign = "center";
        ctx.fillText(n.title, n.x, n.y + 24);
      }
    };

    const loop = () => {
      simulate();
      draw();
      this.anim = requestAnimationFrame(loop);
    };
    loop();
  }
}
