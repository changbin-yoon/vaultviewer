import { useEffect, useMemo, useRef, useState } from "react";
import { forceSimulation, forceLink, forceManyBody, forceCenter, forceCollide, type SimulationNodeDatum } from "d3-force";
import { getVaultIndex, type VaultIndex } from "../lib/vaultIndex";
import { colorForType } from "../lib/typeColors";
import { EmptyState } from "./EmptyState";

interface SimNode extends SimulationNodeDatum {
  id: string;
  name: string;
  resolved: boolean;
  type: string | null;
  degree: number;
}

interface SimEdge {
  source: SimNode;
  target: SimNode;
  relation?: string;
}

const WIDTH = 1000;
const HEIGHT = 640;

function layout(data: VaultIndex): { nodes: SimNode[]; edges: SimEdge[] } {
  const degree = new Map<string, number>();
  for (const e of data.edges) {
    degree.set(e.source, (degree.get(e.source) ?? 0) + 1);
    degree.set(e.target, (degree.get(e.target) ?? 0) + 1);
  }
  const nodes: SimNode[] = data.nodes.map((n) => ({ ...n, degree: degree.get(n.id) ?? 0 }));
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const edges = data.edges
    .filter((e) => byId.has(e.source) && byId.has(e.target))
    .map((e) => ({ source: byId.get(e.source)!, target: byId.get(e.target)!, relation: e.relation }));

  const sim = forceSimulation(nodes)
    .force("charge", forceManyBody().strength(-140))
    .force("link", forceLink(edges).distance(70).strength(0.5))
    .force("center", forceCenter(WIDTH / 2, HEIGHT / 2))
    .force("collide", forceCollide(28))
    .stop();
  for (let i = 0; i < 300; i++) sim.tick();

  return { nodes, edges };
}

export function GraphView({ onNavigate }: { onNavigate: (path: string) => void }) {
  const [data, setData] = useState<VaultIndex | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [hovered, setHovered] = useState<string | null>(null);
  const [selected, setSelected] = useState<SimNode | null>(null);
  const [view, setView] = useState({ scale: 1, x: 0, y: 0 });
  const dragState = useRef<{ startX: number; startY: number; viewX: number; viewY: number } | null>(null);

  useEffect(() => {
    getVaultIndex()
      .then(setData)
      .catch(() => setError("그래프를 불러오지 못했습니다."));
  }, []);

  const { nodes, edges } = useMemo(() => (data ? layout(data) : { nodes: [], edges: [] }), [data]);
  const types = useMemo(
    () => Array.from(new Set(nodes.filter((n) => n.resolved && n.type).map((n) => n.type as string))).sort(),
    [nodes]
  );
  const relationTypes = useMemo(
    () => Array.from(new Set(edges.filter((e) => e.relation).map((e) => e.relation as string))).sort(),
    [edges]
  );
  const relationMarkerId = (relation: string) => `arrow-${relationTypes.indexOf(relation)}`;
  const neighbors = useMemo(() => {
    const m = new Map<string, Set<string>>();
    for (const e of edges) {
      const a = e.source.id, b = e.target.id;
      if (!m.has(a)) m.set(a, new Set());
      if (!m.has(b)) m.set(b, new Set());
      m.get(a)!.add(b);
      m.get(b)!.add(a);
    }
    return m;
  }, [edges]);

  function onWheel(e: React.WheelEvent) {
    e.preventDefault();
    const next = Math.min(3, Math.max(0.3, view.scale * (e.deltaY > 0 ? 0.9 : 1.1)));
    setView((v) => ({ ...v, scale: next }));
  }
  function onPointerDown(e: React.PointerEvent) {
    dragState.current = { startX: e.clientX, startY: e.clientY, viewX: view.x, viewY: view.y };
  }
  function onPointerMove(e: React.PointerEvent) {
    if (!dragState.current) return;
    const d = dragState.current;
    setView((v) => ({ ...v, x: d.viewX + (e.clientX - d.startX), y: d.viewY + (e.clientY - d.startY) }));
  }
  function onPointerUp() {
    dragState.current = null;
  }

  if (error) {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState title={error} />
      </main>
    );
  }
  if (!data) {
    return <main className="p-8 text-sm text-muted">볼트를 스캔하는 중…</main>;
  }
  if (nodes.length === 0) {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState title="표시할 노트가 없습니다" description="마크다운 문서를 만들면 그래프에 나타납니다." />
      </main>
    );
  }

  return (
    <main className="flex flex-col grow min-h-0 relative">
      <div className="px-6 pt-5 pb-3 flex items-center gap-3">
        <h4>그래프 뷰</h4>
        <span className="text-xs text-muted">노트 {data.nodes.filter((n) => n.resolved).length}개 · 링크 {edges.length}개</span>
      </div>
      <div className="absolute top-16 right-6 flex flex-col items-end gap-3">
        {selected && (
          <div
            className="flex flex-col gap-2 p-3 text-xs"
            style={{ background: "var(--color-bg)", border: "1px solid var(--color-divider)", width: 200 }}
          >
            <div className="flex items-center gap-2">
              <span
                style={{
                  width: 9,
                  height: 9,
                  borderRadius: "50%",
                  background: selected.type ? colorForType(selected.type) : "var(--color-accent)",
                  display: "inline-block",
                  flexShrink: 0,
                }}
              />
              <span className="truncate" title={selected.name.replace(/\.md$/, "")}>
                {selected.name.replace(/\.md$/, "")}
              </span>
            </div>
            <div className="flex gap-2">
              <button className="btn btn-primary" style={{ flex: 1 }} onClick={() => onNavigate(selected.id)}>
                페이지 열기
              </button>
              <button className="btn btn-secondary" onClick={() => setSelected(null)}>
                ✕
              </button>
            </div>
            {(() => {
              const incoming = edges.filter((e) => e.relation && e.target.id === selected.id);
              const outgoing = edges.filter((e) => e.relation && e.source.id === selected.id);
              if (incoming.length === 0 && outgoing.length === 0) return null;
              return (
                <div className="flex flex-col gap-2 pt-2 border-t border-[var(--color-divider)]">
                  {incoming.length > 0 && (
                    <div>
                      <div className="text-muted mb-1">영향을 받는 것</div>
                      <div className="flex flex-col gap-1">
                        {incoming.map((e, i) => (
                          <button
                            key={i}
                            className="wikilink text-left"
                            style={{ fontSize: 11 }}
                            onClick={() => setSelected(e.source)}
                          >
                            {e.source.name.replace(/\.md$/, "")}
                            <span className="text-muted"> ({e.relation})</span>
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                  {outgoing.length > 0 && (
                    <div>
                      <div className="text-muted mb-1">이것이 의존하는 것</div>
                      <div className="flex flex-col gap-1">
                        {outgoing.map((e, i) => (
                          <button
                            key={i}
                            className="wikilink text-left"
                            style={{ fontSize: 11 }}
                            onClick={() => setSelected(e.target)}
                          >
                            {e.target.name.replace(/\.md$/, "")}
                            <span className="text-muted"> ({e.relation})</span>
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              );
            })()}
          </div>
        )}
        {types.length > 0 && (
          <div
            className="flex flex-col gap-1.5 p-3 text-xs"
            style={{ background: "var(--color-bg)", border: "1px solid var(--color-divider)" }}
          >
            {types.map((type) => (
              <div key={type} className="flex items-center gap-2">
                <span
                  style={{
                    width: 9,
                    height: 9,
                    borderRadius: "50%",
                    background: colorForType(type),
                    display: "inline-block",
                    flexShrink: 0,
                  }}
                />
                <span>{type}</span>
              </div>
            ))}
          </div>
        )}
        {relationTypes.length > 0 && (
          <div
            className="flex flex-col gap-1.5 p-3 text-xs"
            style={{ background: "var(--color-bg)", border: "1px solid var(--color-divider)" }}
          >
            <div className="text-muted" style={{ fontSize: 10 }}>관계 타입</div>
            {relationTypes.map((relation) => (
              <div key={relation} className="flex items-center gap-2">
                <span style={{ width: 14, height: 2, background: colorForType(relation), display: "inline-block", flexShrink: 0 }} />
                <span>{relation}</span>
              </div>
            ))}
          </div>
        )}
      </div>
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="w-full grow"
        style={{ cursor: dragState.current ? "grabbing" : "grab" }}
        onWheel={onWheel}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerLeave={onPointerUp}
        onClick={() => setSelected(null)}
      >
        <defs>
          {relationTypes.map((relation) => (
            <marker
              key={relation}
              id={relationMarkerId(relation)}
              viewBox="0 0 10 10"
              refX={9}
              refY={5}
              markerWidth={6}
              markerHeight={6}
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill={colorForType(relation)} />
            </marker>
          ))}
        </defs>
        <g transform={`translate(${view.x} ${view.y}) scale(${view.scale})`}>
          {edges.map((e, i) => {
            const dim = hovered && !neighbors.get(hovered)?.has(e.source.id === hovered ? e.target.id : e.source.id);
            return (
              <line
                key={i}
                x1={e.source.x}
                y1={e.source.y}
                x2={e.target.x}
                y2={e.target.y}
                stroke={e.relation ? colorForType(e.relation) : "var(--color-divider)"}
                strokeWidth={e.relation ? 1.5 : 1}
                opacity={dim ? 0.15 : e.relation ? 0.85 : 0.7}
                markerEnd={e.relation ? `url(#${relationMarkerId(e.relation)})` : undefined}
              >
                {e.relation && <title>{e.relation}</title>}
              </line>
            );
          })}
          {nodes.map((n) => {
            const r = n.resolved ? 4 + Math.min(10, n.degree * 1.4) : 3;
            const dim = hovered && hovered !== n.id && !neighbors.get(hovered)?.has(n.id);
            return (
              <g
                key={n.id}
                transform={`translate(${n.x} ${n.y})`}
                opacity={dim ? 0.25 : 1}
                style={{ cursor: n.resolved ? "pointer" : "default" }}
                onMouseEnter={() => setHovered(n.id)}
                onMouseLeave={() => setHovered(null)}
                onClick={(e) => {
                  if (!n.resolved) return;
                  e.stopPropagation();
                  setSelected(n);
                }}
              >
                <circle
                  r={r}
                  fill={n.resolved ? (n.type ? colorForType(n.type) : "var(--color-accent)") : "var(--color-bg)"}
                  stroke={n.resolved ? "none" : "var(--color-divider)"}
                  strokeDasharray={n.resolved ? undefined : "2,2"}
                />
                <text x={r + 5} y={4} fontSize={11} fontFamily="var(--font-body)" fill="var(--color-text)">
                  {n.name.replace(/\.md$/, "")}
                </text>
              </g>
            );
          })}
        </g>
      </svg>
    </main>
  );
}
