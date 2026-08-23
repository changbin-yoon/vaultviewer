import { useEffect, useState } from "react";
import * as api from "../lib/api";
import type { FileItem } from "../lib/api";
import { isManagedFolderName, isImageTarget, stripMdExtension } from "../lib/markdown";
import { useAuth, canWrite } from "../lib/auth";

interface NodeState {
  children: FileItem[] | null; // null = not fetched yet
  expanded: boolean;
}

// Images are meant to be viewed inline inside notes (via embeds), not
// browsed as standalone tree entries — hide them here regardless of which
// folder they happen to sit in, alongside the dedicated attachments/
// folder itself.
function isHiddenFromTree(item: FileItem): boolean {
  if (item.isDir) return isManagedFolderName(item.name);
  return isImageTarget(item.name);
}

function subtreeMatches(item: FileItem, filter: string, byPath: Record<string, NodeState>): boolean {
  if (item.name.toLowerCase().includes(filter)) return true;
  const state = byPath[item.path];
  if (!state?.children) return false;
  return state.children.some((child) => subtreeMatches(child, filter, byPath));
}

export function Tree({
  selectedPath,
  onSelect,
  onCollapse,
  refreshSignal,
}: {
  selectedPath: string | null;
  onSelect: (path: string) => void;
  onCollapse?: () => void;
  // Bumped by the parent whenever a note/namespace is created or deleted
  // elsewhere (the main panel, not this tree) — those don't touch this
  // component's state directly, so without this the tree would keep
  // showing whatever it last fetched. On change, re-fetch the root and
  // every currently-expanded folder (not the whole cache blindly — a
  // collapsed folder's stale entry is harmless since it re-fetches the
  // next time it's opened anyway).
  refreshSignal?: number;
}) {
  const { session } = useAuth();
  const role = session!.role;
  const [rootItems, setRootItems] = useState<FileItem[] | null>(null);
  const [byPath, setByPath] = useState<Record<string, NodeState>>({});
  const [filter, setFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [creatingNamespace, setCreatingNamespace] = useState(false);
  const [newNamespaceName, setNewNamespaceName] = useState("");
  const [namespaceError, setNamespaceError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function refreshRoot() {
    api
      .listTree("")
      .then((items) => setRootItems(items ?? []))
      .catch(() => setError("트리를 불러오지 못했습니다."));
  }

  useEffect(refreshRoot, []);

  useEffect(() => {
    if (!refreshSignal) return; // 0/undefined: initial mount, nothing to refresh yet
    refreshRoot();
    for (const [path, node] of Object.entries(byPath)) {
      if (!node.expanded) continue;
      api
        .listTree(path)
        .then((children) => setByPath((prev) => ({ ...prev, [path]: { ...prev[path], children: children ?? [] } })))
        .catch(() => {});
    }
    // Only react to refreshSignal itself — byPath is read for its value at
    // that moment, not to re-run this on every expand/collapse.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshSignal]);

  async function submitNamespace() {
    const trimmed = newNamespaceName.trim();
    if (!trimmed) return;
    setBusy(true);
    setNamespaceError(null);
    try {
      await api.createNamespace(trimmed);
      setCreatingNamespace(false);
      setNewNamespaceName("");
      refreshRoot();
    } catch {
      setNamespaceError("네임스페이스 생성에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function toggle(item: FileItem) {
    const state = byPath[item.path];
    if (state?.children == null) {
      try {
        const children = await api.listTree(item.path);
        setByPath((prev) => ({ ...prev, [item.path]: { children: children ?? [], expanded: true } }));
      } catch {
        setByPath((prev) => ({ ...prev, [item.path]: { children: [], expanded: true } }));
      }
    } else {
      setByPath((prev) => ({ ...prev, [item.path]: { ...state, expanded: !state.expanded } }));
    }
  }

  function onRowClick(item: FileItem) {
    onSelect(item.path);
    if (item.isDir) toggle(item);
  }

  const activeFilter = filter.trim().toLowerCase();

  function renderNode(item: FileItem, depth: number): React.ReactNode {
    if (isHiddenFromTree(item)) return null;
    if (activeFilter && !subtreeMatches(item, activeFilter, byPath)) return null;

    const state = byPath[item.path];
    const expanded = activeFilter ? true : !!state?.expanded;
    const isSelected = item.path === selectedPath;

    return (
      <div key={item.path}>
        <button
          className={`tree-row${isSelected ? " selected" : ""}`}
          style={{ paddingLeft: 20 + depth * 18 }}
          onClick={() => onRowClick(item)}
        >
          {item.isDir ? <span className="opacity-60">{expanded ? "▾" : "▸"}</span> : null}
          {stripMdExtension(item.name)}
        </button>
        {item.isDir && expanded && state?.children?.map((child) => renderNode(child, depth + 1))}
      </div>
    );
  }

  return (
    <nav className="border-r border-[var(--color-divider)] py-4 text-[13.5px] overflow-y-auto">
      <div className="flex items-center justify-between px-5 pb-2.5">
        <span className="text-[10px] tracking-[.12em] uppercase text-muted">네임스페이스</span>
        <div className="flex items-center gap-2">
          {rootItems && (
            <span className="mono text-[11px] text-muted">
              {rootItems.filter((i) => !isHiddenFromTree(i)).length}
            </span>
          )}
          {canWrite(role) && (
            <button
              className="btn btn-ghost"
              style={{ fontSize: 14, lineHeight: 1, padding: "0 4px" }}
              title="새 네임스페이스"
              onClick={() => setCreatingNamespace((v) => !v)}
            >
              +
            </button>
          )}
          {onCollapse && (
            <button
              className="btn btn-ghost"
              style={{ fontSize: 14, lineHeight: 1, padding: "0 4px" }}
              title="사이드바 접기"
              onClick={onCollapse}
            >
              ◂
            </button>
          )}
        </div>
      </div>
      {creatingNamespace && (
        <div className="px-5 pb-2.5">
          <input
            className="input mono text-[13px] mb-1.5"
            placeholder="네임스페이스 이름"
            value={newNamespaceName}
            onChange={(e) => setNewNamespaceName(e.target.value)}
            autoFocus
          />
          {namespaceError && <p className="text-xs mb-1.5" style={{ color: "#b3432f" }}>{namespaceError}</p>}
          <div className="flex gap-1.5">
            <button className="btn btn-primary text-xs" disabled={busy || !newNamespaceName.trim()} onClick={submitNamespace}>
              만들기
            </button>
            <button className="btn btn-secondary text-xs" onClick={() => setCreatingNamespace(false)}>
              취소
            </button>
          </div>
        </div>
      )}
      <div className="px-5 pb-2.5">
        <input
          className="input text-[13px]"
          placeholder="불러온 항목에서 검색"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>
      {error && <p className="px-5 text-sm text-muted">{error}</p>}
      {rootItems?.map((item) => renderNode(item, 0))}
      {rootItems?.length === 0 && <p className="px-5 text-sm text-muted">항목이 없습니다.</p>}
    </nav>
  );
}
