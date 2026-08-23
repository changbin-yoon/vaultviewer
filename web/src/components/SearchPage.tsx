import { useEffect, useState } from "react";
import * as api from "../lib/api";
import type { SearchResult } from "../lib/api";
import { EmptyState } from "./EmptyState";
import { stripMdExtension } from "../lib/markdown";

function highlight(snippet: string, query: string) {
  if (!query.trim()) return snippet;
  const idx = snippet.toLowerCase().indexOf(query.toLowerCase());
  if (idx < 0) return snippet;
  return (
    <>
      {snippet.slice(0, idx)}
      <mark className="bg-transparent text-[var(--color-accent)] font-semibold">
        {snippet.slice(idx, idx + query.length)}
      </mark>
      {snippet.slice(idx + query.length)}
    </>
  );
}

export function SearchPage({ onNavigate }: { onNavigate: (path: string) => void }) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const trimmed = query.trim();
    if (!trimmed) {
      setResults(null);
      setError(null);
      return;
    }
    setLoading(true);
    const timer = setTimeout(() => {
      api
        .search(trimmed)
        .then((r) => {
          setResults(r ?? []);
          setError(null);
        })
        .catch(() => setError("검색에 실패했습니다."))
        .finally(() => setLoading(false));
    }, 250);
    return () => clearTimeout(timer);
  }, [query]);

  return (
    <main className="p-6 max-w-2xl">
      <h4 className="mb-4">전체 텍스트 검색</h4>
      <input
        className="input text-[14px] mb-6"
        placeholder="볼트 전체에서 검색"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        autoFocus
      />

      {error && <p className="text-sm text-muted">{error}</p>}

      {!error && !query.trim() && (
        <EmptyState title="검색어를 입력하세요" description="노트 본문 전체에서 일치하는 내용을 찾습니다." />
      )}

      {!error && query.trim() && loading && <p className="text-sm text-muted">검색 중…</p>}

      {!error && query.trim() && !loading && results && results.length === 0 && (
        <EmptyState title="일치하는 결과가 없습니다" description={`"${query.trim()}"와(과) 일치하는 노트를 찾지 못했습니다.`} />
      )}

      {!error && !loading && results && results.length > 0 && (
        <div className="flex flex-col gap-1">
          <p className="text-xs text-muted mb-2">{results.length}개 결과</p>
          {results.map((r) => (
            <button
              key={r.path}
              className="text-left p-3 border border-[var(--color-divider)]"
              onClick={() => onNavigate(r.path)}
            >
              <div className="wikilink text-sm mono mb-1">{stripMdExtension(r.path)}</div>
              <div className="text-sm text-muted">{highlight(r.snippet, query.trim())}</div>
            </button>
          ))}
        </div>
      )}
    </main>
  );
}
