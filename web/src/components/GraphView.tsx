import React from "react";
import type { Triple } from "@/lib/types";

interface GraphViewProps {
  triples: Triple[];
  total: number;
}

interface GraphNode {
  id: string;
  label: string;
  x: number;
  y: number;
  connections: number;
}

interface Edge {
  source: string;
  target: string;
  predicate: string;
  confidence: number;
}

interface Layout {
  nodes: GraphNode[];
  edges: Edge[];
}

function computeLayout(triples: Triple[]): Layout {
  const nodeMap = new Map<string, GraphNode>();
  const edges: Edge[] = [];
  const W = 800, H = 500, cx = W / 2, cy = H / 2;

  for (const t of triples) {
    if (!nodeMap.has(t.subject)) {
      nodeMap.set(t.subject, { id: t.subject, label: t.subject, x: 0, y: 0, connections: 0 });
    }
    if (!nodeMap.has(t.object)) {
      nodeMap.set(t.object, { id: t.object, label: t.object, x: 0, y: 0, connections: 0 });
    }
    nodeMap.get(t.subject)!.connections++;
    nodeMap.get(t.object)!.connections++;
    edges.push({ source: t.subject, target: t.object, predicate: t.predicate, confidence: t.confidence });
  }

  const nodes = Array.from(nodeMap.values());

  // place in golden-angle spiral
  for (let i = 0; i < nodes.length; i++) {
    const angle = i * 2.399963; // golden angle
    const r = 60 + i * 18;
    nodes[i].x = cx + r * Math.cos(angle);
    nodes[i].y = cy + r * Math.sin(angle);
  }

  // run force simulation synchronously (no re-renders)
  for (let iter = 0; iter < 200; iter++) {
    const alpha = 1 - iter / 200;
    for (const n of nodes) {
      let fx = (cx - n.x) * 0.01;
      let fy = (cy - n.y) * 0.01;
      for (const other of nodes) {
        if (other.id === n.id) continue;
        const dx = n.x - other.x;
        const dy = n.y - other.y;
        const d2 = dx * dx + dy * dy || 1;
        const repulse = 4000 / d2;
        const dist = Math.sqrt(d2);
        fx += (dx / dist) * repulse;
        fy += (dy / dist) * repulse;
      }
      for (const e of edges) {
        let other: GraphNode | undefined;
        if (e.source === n.id) other = nodeMap.get(e.target);
        else if (e.target === n.id) other = nodeMap.get(e.source);
        if (!other) continue;
        const dx = other.x - n.x;
        const dy = other.y - n.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const attract = (dist - 120) * 0.005;
        fx += (dx / dist) * attract;
        fy += (dy / dist) * attract;
      }
      n.x += fx * alpha * 0.6;
      n.y += fy * alpha * 0.6;
      n.x = Math.max(50, Math.min(W - 50, n.x));
      n.y = Math.max(50, Math.min(H - 50, n.y));
    }
  }

  return { nodes, edges };
}

const predicateColors: Record<string, string> = {
  depends_on: "#f97316",
  uses: "#22c55e",
  deployed_on: "#3b82f6",
  owns: "#a855f7",
  integrates_with: "#eab308",
  relates_to: "#6b7280",
};

// Stable string hash → hue. Same predicate always gets the same color, so
// unknown ones still have visual identity instead of all collapsing to one
// fallback shade.
function hashHue(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) | 0;
  }
  return ((h % 360) + 360) % 360;
}

function predicateColor(p: string): string {
  return predicateColors[p] || `hsl(${hashHue(p)} 65% 55%)`;
}

function trunc(s: string, n: number): string {
  return s.length <= n ? s : s.slice(0, n - 1) + "…";
}

function nodeR(c: number): number {
  return Math.min(22, Math.max(8, 6 + c * 2));
}

export function GraphView({ triples, total }: GraphViewProps) {
  const svgRef = React.useRef<SVGSVGElement>(null);
  const layout = React.useMemo(() => computeLayout(triples), [triples]);
  const [pos, setPos] = React.useState<Record<string, { x: number; y: number }>>({});
  const [hovered, setHovered] = React.useState<string | null>(null);
  const [hoveredEdge, setHoveredEdge] = React.useState<number | null>(null);
  const [dragging, setDragging] = React.useState<string | null>(null);
  const dragOff = React.useRef({ x: 0, y: 0 });

  React.useEffect(() => {
    const p: Record<string, { x: number; y: number }> = {};
    for (const n of layout.nodes) p[n.id] = { x: n.x, y: n.y };
    setPos(p);
  }, [layout]);

  if (triples.length === 0) {
    return (
      <div style={{ padding: "60px 22px", textAlign: "center", color: "var(--text-dim)", fontSize: 13 }}>
        <div style={{ marginBottom: 8 }}>
          <svg width="48" height="48" viewBox="0 0 48 48" style={{ opacity: 0.3 }}>
            <circle cx="16" cy="24" r="6" fill="none" stroke="currentColor" strokeWidth="1.5" />
            <circle cx="32" cy="24" r="6" fill="none" stroke="currentColor" strokeWidth="1.5" />
            <circle cx="24" cy="12" r="6" fill="none" stroke="currentColor" strokeWidth="1.5" />
            <line x1="21" y1="16" x2="19" y2="20" stroke="currentColor" strokeWidth="1" opacity="0.4" />
            <line x1="27" y1="16" x2="29" y2="20" stroke="currentColor" strokeWidth="1" opacity="0.4" />
          </svg>
        </div>
        No knowledge graph data yet.
        <div style={{ marginTop: 6, fontSize: 12, color: "var(--text-ghost)" }}>
          Graph triples will appear here when synced from Anchored clients.
        </div>
      </div>
    );
  }

  function onDown(e: React.PointerEvent, id: string) {
    setDragging(id);
    const svg = svgRef.current!;
    const rect = svg.getBoundingClientRect();
    const px = (e.clientX - rect.left) / rect.width * 800;
    const py = (e.clientY - rect.top) / rect.height * 500;
    dragOff.current = { x: px - pos[id].x, y: py - pos[id].y };
    (e.target as Element).setPointerCapture(e.pointerId);
  }

  function onMove(e: React.PointerEvent) {
    if (!dragging) return;
    const svg = svgRef.current!;
    const rect = svg.getBoundingClientRect();
    const px = (e.clientX - rect.left) / rect.width * 800;
    const py = (e.clientY - rect.top) / rect.height * 500;
    setPos(prev => ({ ...prev, [dragging]: { x: Math.max(50, Math.min(750, px - dragOff.current.x)), y: Math.max(50, Math.min(450, py - dragOff.current.y)) } }));
  }

  function onUp() { setDragging(null); }

  const uniquePreds = [...new Set(triples.map(t => t.predicate))];

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 14, fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
        <span><span style={{ color: "var(--text)" }}>{layout.nodes.length}</span> nodes</span>
        <span style={{ color: "var(--text-ghost)" }}>·</span>
        <span><span style={{ color: "var(--text)" }}>{layout.edges.length}</span> edges</span>
        <span style={{ color: "var(--text-ghost)" }}>·</span>
        <span><span style={{ color: "var(--text)" }}>{uniquePreds.length}</span> predicates</span>
        <span style={{ color: "var(--text-ghost)" }}>·</span>
        <span><span style={{ color: "var(--text)" }}>{total.toLocaleString()}</span> total</span>
        {total > triples.length && (
          <>
            <span style={{ color: "var(--text-ghost)" }}>·</span>
            <span style={{ color: "var(--warn, #d97706)" }}>showing first {triples.length} — paginate for the rest</span>
          </>
        )}
        <div style={{ flex: 1 }} />
        <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
          {uniquePreds.slice(0, 6).map(p => (
            <span key={p} style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
              <span style={{ width: 8, height: 8, borderRadius: "50%", background: predicateColor(p) }} />
              <span>{p.replace(/_/g, " ")}</span>
            </span>
          ))}
        </div>
      </div>

      <div style={{ background: "var(--bg-1)", borderRadius: "var(--radius)", border: "1px solid var(--border)", overflow: "hidden" }}>
        <svg ref={svgRef} viewBox="0 0 800 500" style={{ width: "100%", height: 500, display: "block" }} onPointerMove={onMove} onPointerUp={onUp}>
          <defs>
            <marker id="ah" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
              <polygon points="0 0, 8 3, 0 6" fill="var(--text-dim)" opacity="0.5" />
            </marker>
          </defs>

          {layout.edges.map((e, i) => {
            const s = pos[e.source], t = pos[e.target];
            if (!s || !t) return null;
            const dx = t.x - s.x, dy = t.y - s.y;
            const dist = Math.sqrt(dx * dx + dy * dy) || 1;
            const tgt = layout.nodes.find(n => n.id === e.target);
            const r = nodeR(tgt?.connections || 1);
            const ex = t.x - (dx / dist) * (r + 6), ey = t.y - (dy / dist) * (r + 6);
            const isH = hoveredEdge === i;
            const isDimmed = hovered != null && e.source !== hovered && e.target !== hovered;
            // Confidence ∈ [0, 1] modulates stroke width + base opacity so
            // low-confidence edges visually recede.
            const conf = Math.max(0, Math.min(1, e.confidence ?? 1));
            const baseOpacity = 0.18 + 0.32 * conf;
            const baseWidth = 0.6 + 1.6 * conf;
            return (
              <g key={i}>
                <line x1={s.x} y1={s.y} x2={ex} y2={ey}
                  stroke={predicateColor(e.predicate)}
                  strokeWidth={isH ? baseWidth + 1 : baseWidth}
                  opacity={isDimmed ? 0.06 : isH ? Math.min(1, baseOpacity + 0.5) : baseOpacity}
                  markerEnd="url(#ah)"
                  onPointerEnter={() => setHoveredEdge(i)}
                  onPointerLeave={() => setHoveredEdge(null)}
                  style={{ cursor: "pointer" }}
                />
                {(isH || isDimmed === false) && (
                  <text
                    x={(s.x + ex) / 2} y={(s.y + ey) / 2 - 6}
                    textAnchor="middle" fill={predicateColor(e.predicate)}
                    fontSize="10" fontFamily="var(--font-mono)"
                    opacity={isH ? 1 : 0.6}
                    style={{ pointerEvents: "none" }}
                  >{e.predicate.replace(/_/g, " ")}</text>
                )}
              </g>
            );
          })}

          {layout.nodes.map(n => {
            const p = pos[n.id];
            if (!p) return null;
            const r = nodeR(n.connections);
            const isH = hovered === n.id;
            const isConn = hovered == null || layout.edges.some(e => (e.source === hovered && e.target === n.id) || (e.target === hovered && e.source === n.id));
            return (
              <g key={n.id}
                onPointerEnter={() => setHovered(n.id)}
                onPointerLeave={() => setHovered(null)}
                onPointerDown={(e) => onDown(e, n.id)}
                style={{ cursor: "grab" }}
              >
                <circle cx={p.x} cy={p.y} r={r + 4} fill="transparent" />
                <circle cx={p.x} cy={p.y} r={r}
                  fill={isH ? "var(--accent)" : isConn ? "var(--bg-2)" : "var(--bg-1)"}
                  stroke={isH ? "var(--accent)" : isConn ? "var(--text-dim)" : "var(--border)"}
                  strokeWidth={isH ? 2 : 1}
                  opacity={isConn ? 1 : 0.25}
                />
                <text x={p.x} y={p.y + r + 13} textAnchor="middle"
                  fill={isH ? "var(--accent)" : isConn ? "var(--text)" : "var(--text-dim)"}
                  fontSize="10" fontFamily="var(--font-mono)"
                  opacity={isConn ? 1 : 0.3}
                >{trunc(n.label, 22)}</text>
              </g>
            );
          })}
        </svg>
      </div>

      <div style={{ marginTop: 14, fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
        drag nodes to rearrange · hover for details
      </div>
    </div>
  );
}
