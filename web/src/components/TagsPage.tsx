import { useEffect, useState } from "react";
import { getVaultIndex, type VaultIndex } from "../lib/vaultIndex";
import { EmptyState } from "./EmptyState";
import { stripMdExtension } from "../lib/markdown";

export function TagsPage({ onNavigate }: { onNavigate: (path: string) => void }) {
  const [index, setIndex] = useState<VaultIndex | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    getVaultIndex()
      .then(setIndex)
      .catch(() => setError("태그를 불러오지 못했습니다."));
  }, []);

  if (error) {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState title={error} />
      </main>
    );
  }
  if (!index) {
    return <main className="p-8 text-sm text-muted">볼트를 스캔하는 중…</main>;
  }

  const tags = Array.from(index.tags.entries()).sort((a, b) => b[1].length - a[1].length || a[0].localeCompare(b[0]));

  if (tags.length === 0) {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState title="태그가 없습니다" description="프론트매터의 tags: 항목이나 본문의 #태그를 사용하면 여기 모입니다." />
      </main>
    );
  }

  return (
    <main className="p-6 grid" style={{ gridTemplateColumns: "260px 1fr", gap: 32 }}>
      <div>
        <h4 className="mb-4">태그 {tags.length}개</h4>
        <div className="flex flex-wrap gap-2">
          {tags.map(([tag, paths]) => (
            <button
              key={tag}
              className={`tag mono${selected === tag ? " tag-accent" : " tag-neutral"}`}
              style={{ cursor: "pointer" }}
              onClick={() => setSelected(tag)}
            >
              #{tag} <span className="opacity-60 ml-1">{paths.length}</span>
            </button>
          ))}
        </div>
      </div>
      <div>
        {selected ? (
          <>
            <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">
              #{selected}이(가) 붙은 노트
            </h5>
            <div className="flex flex-col gap-1">
              {index.tags.get(selected)!.map((p) => (
                <button key={p} className="wikilink text-left text-sm" onClick={() => onNavigate(p)}>
                  {stripMdExtension(p)}
                </button>
              ))}
            </div>
          </>
        ) : (
          <p className="text-sm text-muted">왼쪽에서 태그를 선택하세요.</p>
        )}
      </div>
    </main>
  );
}
