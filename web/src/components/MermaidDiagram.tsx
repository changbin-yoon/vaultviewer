import { useEffect, useRef, useState } from "react";
import mermaid from "mermaid";

let initialized = false;
function ensureInit() {
  if (initialized) return;
  mermaid.initialize({ startOnLoad: false, theme: "neutral", securityLevel: "strict" });
  initialized = true;
}

let counter = 0;

export function MermaidDiagram({ code }: { code: string }) {
  const [svg, setSvg] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const id = useRef(`mermaid-${++counter}`);

  useEffect(() => {
    let cancelled = false;
    ensureInit();
    mermaid
      .render(id.current, code)
      .then((result) => {
        if (!cancelled) setSvg(result.svg);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [code]);

  if (error) {
    return (
      <div className="callout" style={{ borderColor: "var(--color-danger)" }}>
        <div className="mb-1.5">
          <span className="tag tag-outline mono">MERMAID 오류</span>
        </div>
        <pre className="mono m-0 whitespace-pre-wrap text-sm">{error}</pre>
      </div>
    );
  }
  if (!svg) return <p className="text-sm text-muted">다이어그램 렌더링 중…</p>;
  return <div className="mermaid-diagram" dangerouslySetInnerHTML={{ __html: svg }} />;
}
