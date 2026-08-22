import { useEffect, useState } from "react";
import * as api from "../lib/api";
import type { AuditLog, FileItem } from "../lib/api";
import { useAuth, canWrite, canDelete } from "../lib/auth";
import { EmptyState } from "./EmptyState";
import { MarkdownDocument } from "./MarkdownDocument";
import { isManagedFolderName, isImageTarget, stripMdExtension } from "../lib/markdown";

interface KeyRow {
  path: string;
  name: string;
  content: string;
  latest: AuditLog | null;
}

type ViewState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "browse"; groupPath: string; children: FileItem[] }
  | { kind: "empty"; groupPath: string }
  | { kind: "secret"; groupPath: string; rows: KeyRow[] };

const ACTION_LABEL: Record<string, string> = { create: "추가", update: "수정", delete: "삭제" };

async function loadKeyRow(item: FileItem): Promise<KeyRow> {
  const [file, history] = await Promise.all([
    api.readFile(item.path),
    api.getHistory(item.path).catch(() => []),
  ]);
  const latest = history && history.length > 0 ? history[history.length - 1] : null;
  return { path: item.path, name: item.name, content: api.decodeContent(file.content), latest };
}

export function SecretPanel({ selectedPath, onNavigate }: { selectedPath: string | null; onNavigate: (path: string) => void }) {
  const { session } = useAuth();
  const role = session!.role;
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [revealed, setRevealed] = useState(false);
  const [editing, setEditing] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [reason, setReason] = useState("");
  const [addingKey, setAddingKey] = useState(false);
  const [newKeyName, setNewKeyName] = useState("");
  const [newKeyValue, setNewKeyValue] = useState("");
  const [creatingNote, setCreatingNote] = useState(false);
  const [newNoteName, setNewNoteName] = useState("");
  const [creatingNamespace, setCreatingNamespace] = useState(false);
  const [newNamespaceName, setNewNamespaceName] = useState("");
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    setEditing(false);
    setAddingKey(false);
    setCreatingNote(false);
    setNewNoteName("");
    setCreatingNamespace(false);
    setNewNamespaceName("");
    setActionError(null);
    setRevealed(false);
    if (selectedPath == null || selectedPath.endsWith(".md")) return;
    void load(selectedPath);
  }, [selectedPath]);

  async function load(path: string) {
    setState({ kind: "loading" });
    try {
      const children = await api.listTree(path);
      const items = children ?? [];
      if (items.length === 0) {
        setState({ kind: "empty", groupPath: path });
        return;
      }
      if (items.some((c) => c.isDir || c.name.endsWith(".md"))) {
        setState({ kind: "browse", groupPath: path, children: items });
        return;
      }
      const rows = await Promise.all(items.map(loadKeyRow));
      setState({ kind: "secret", groupPath: path, rows });
    } catch {
      // listing failed: this path is itself a leaf key, not a directory.
      try {
        const segments = path.split("/").filter(Boolean);
        const name = segments[segments.length - 1] ?? path;
        const row = await loadKeyRow({ path, name, isDir: false });
        const groupPath = segments.slice(0, -1).join("/");
        setState({ kind: "secret", groupPath, rows: [row] });
      } catch {
        setState({ kind: "error", message: "이 경로를 불러올 수 없습니다." });
      }
    }
  }

  async function submitNewNote(groupPath: string) {
    const trimmed = newNoteName.trim();
    if (!trimmed) return;
    const fileName = trimmed.endsWith(".md") ? trimmed : `${trimmed}.md`;
    setBusy(true);
    setActionError(null);
    try {
      const path = groupPath ? `${groupPath}/${fileName}` : fileName;
      await api.saveFile(path, "", "새 노트 생성");
      setCreatingNote(false);
      setNewNoteName("");
      onNavigate(path);
    } catch {
      setActionError("노트 생성에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function submitNewNamespace(groupPath: string) {
    const trimmed = newNamespaceName.trim();
    if (!trimmed) return;
    setBusy(true);
    setActionError(null);
    try {
      const path = groupPath ? `${groupPath}/${trimmed}` : trimmed;
      await api.createNamespace(path);
      setCreatingNamespace(false);
      setNewNamespaceName("");
      onNavigate(path);
    } catch {
      setActionError("네임스페이스 생성에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  if (selectedPath == null) {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState title="왼쪽에서 시크릿을 선택하세요" description="네임스페이스를 펼쳐 경로를 탐색할 수 있습니다." />
      </main>
    );
  }
  if (selectedPath.endsWith(".md")) {
    return <MarkdownDocument path={selectedPath} onNavigate={onNavigate} />;
  }
  if (state.kind === "loading") {
    return <main className="p-8 text-sm text-muted">불러오는 중…</main>;
  }
  if (state.kind === "error") {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState title={state.message} />
      </main>
    );
  }
  if (state.kind === "browse") {
    const writeHint = !canWrite(role) ? "view 역할은 수정할 수 없습니다" : "";
    return (
      <main className="p-8">
        <div className="flex items-start gap-3.5 mb-1">
          <div className="mono text-xs text-muted">/{state.groupPath}</div>
          <div className="ml-auto flex gap-2">
            <button
              className="btn btn-secondary"
              disabled={!canWrite(role)}
              title={writeHint}
              onClick={() => setCreatingNamespace((v) => !v)}
            >
              새 네임스페이스
            </button>
            <button
              className="btn btn-secondary"
              disabled={!canWrite(role)}
              title={writeHint}
              onClick={() => setCreatingNote((v) => !v)}
            >
              새 노트
            </button>
          </div>
        </div>
        {creatingNamespace && (
          <div className="blueprint p-5 my-5">
            <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
            <div className="field mb-4">
              <label>네임스페이스 이름</label>
              <input
                className="input mono"
                value={newNamespaceName}
                onChange={(e) => setNewNamespaceName(e.target.value)}
                placeholder="예: 새-네임스페이스"
                autoFocus
              />
            </div>
            {actionError && <p className="text-sm mb-3" style={{ color: "#b3432f" }}>{actionError}</p>}
            <div className="flex gap-2.5">
              <button
                className="btn btn-primary"
                disabled={busy || !newNamespaceName.trim()}
                onClick={() => submitNewNamespace(state.groupPath)}
              >
                만들기
              </button>
              <button className="btn btn-secondary" onClick={() => setCreatingNamespace(false)}>취소</button>
            </div>
          </div>
        )}
        {creatingNote && (
          <div className="blueprint p-5 my-5">
            <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
            <div className="field mb-4">
              <label>노트 이름</label>
              <input
                className="input mono"
                value={newNoteName}
                onChange={(e) => setNewNoteName(e.target.value)}
                placeholder="예: 새-노트"
                autoFocus
              />
            </div>
            {actionError && <p className="text-sm mb-3" style={{ color: "#b3432f" }}>{actionError}</p>}
            <div className="flex gap-2.5">
              <button
                className="btn btn-primary"
                disabled={busy || !newNoteName.trim()}
                onClick={() => submitNewNote(state.groupPath)}
              >
                만들기
              </button>
              <button className="btn btn-secondary" onClick={() => setCreatingNote(false)}>취소</button>
            </div>
          </div>
        )}
        <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5 mt-5">하위 경로를 선택하세요</h5>
        <div className="flex flex-col">
          {state.children
            .filter((c) => !(c.isDir ? isManagedFolderName(c.name) : isImageTarget(c.name)))
            .map((c) => (
              <button
                key={c.path}
                className="tree-row !px-0 !justify-start"
                onClick={() => onNavigate(c.path)}
              >
                {c.isDir ? "▸" : ""} {stripMdExtension(c.name)}
              </button>
            ))}
        </div>
      </main>
    );
  }
  if (state.kind === "empty") {
    return (
      <main className="p-8 flex items-center justify-center">
        <EmptyState
          title={<span className="mono">/{state.groupPath}</span>}
          description="이 경로에 아직 아무것도 없습니다."
          action={
            canWrite(role) && (
              <div className="flex gap-2.5 justify-center">
                {creatingNote ? (
                  <div className="blueprint p-5 text-left" style={{ minWidth: 280 }}>
                    <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
                    <div className="field mb-4">
                      <label>노트 이름</label>
                      <input
                        className="input mono"
                        value={newNoteName}
                        onChange={(e) => setNewNoteName(e.target.value)}
                        placeholder="예: 새-노트"
                        autoFocus
                      />
                    </div>
                    {actionError && <p className="text-sm mb-3" style={{ color: "#b3432f" }}>{actionError}</p>}
                    <div className="flex gap-2.5">
                      <button
                        className="btn btn-primary"
                        disabled={busy || !newNoteName.trim()}
                        onClick={() => submitNewNote(state.groupPath)}
                      >
                        만들기
                      </button>
                      <button className="btn btn-secondary" onClick={() => setCreatingNote(false)}>취소</button>
                    </div>
                  </div>
                ) : addingKey ? (
                  <div className="blueprint p-5 text-left" style={{ minWidth: 280 }}>
                    <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
                    <div className="field mb-3.5">
                      <label>키 이름</label>
                      <input className="input mono" value={newKeyName} onChange={(e) => setNewKeyName(e.target.value)} autoFocus />
                    </div>
                    <div className="field mb-4">
                      <label>값</label>
                      <textarea
                        className="input mono"
                        style={{ resize: "vertical" }}
                        rows={3}
                        value={newKeyValue}
                        onChange={(e) => setNewKeyValue(e.target.value)}
                      />
                    </div>
                    {actionError && <p className="text-sm mb-3" style={{ color: "#b3432f" }}>{actionError}</p>}
                    <div className="flex gap-2.5">
                      <button
                        className="btn btn-primary"
                        disabled={busy || !newKeyName.trim()}
                        onClick={() => submitNewKey(state.groupPath)}
                      >
                        추가
                      </button>
                      <button className="btn btn-secondary" onClick={() => setAddingKey(false)}>취소</button>
                    </div>
                  </div>
                ) : creatingNamespace ? (
                  <div className="blueprint p-5 text-left" style={{ minWidth: 280 }}>
                    <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
                    <div className="field mb-4">
                      <label>네임스페이스 이름</label>
                      <input
                        className="input mono"
                        value={newNamespaceName}
                        onChange={(e) => setNewNamespaceName(e.target.value)}
                        placeholder="예: 새-네임스페이스"
                        autoFocus
                      />
                    </div>
                    {actionError && <p className="text-sm mb-3" style={{ color: "#b3432f" }}>{actionError}</p>}
                    <div className="flex gap-2.5">
                      <button
                        className="btn btn-primary"
                        disabled={busy || !newNamespaceName.trim()}
                        onClick={() => submitNewNamespace(state.groupPath)}
                      >
                        만들기
                      </button>
                      <button className="btn btn-secondary" onClick={() => setCreatingNamespace(false)}>취소</button>
                    </div>
                  </div>
                ) : (
                  <>
                    <button className="btn btn-primary" onClick={() => setCreatingNamespace(true)}>
                      새 네임스페이스
                    </button>
                    <button className="btn btn-secondary" onClick={() => setCreatingNote(true)}>
                      새 노트 만들기
                    </button>
                    <button className="btn btn-secondary" onClick={() => setAddingKey(true)}>
                      키·값 만들기
                    </button>
                  </>
                )}
              </div>
            )
          }
        />
      </main>
    );
  }

  const { groupPath, rows } = state;
  const writeHint = !canWrite(role) ? "view 역할은 수정할 수 없습니다" : "";
  const deleteHint = !canDelete(role) ? "삭제는 adm 역할만 가능합니다" : "";

  function startEdit() {
    setDrafts(Object.fromEntries(rows.map((r) => [r.path, r.content])));
    setReason("");
    setRevealed(true);
    setEditing(true);
  }

  async function saveEdits() {
    setBusy(true);
    setActionError(null);
    try {
      const changed = rows.filter((r) => drafts[r.path] !== r.content);
      await Promise.all(changed.map((r) => api.saveFile(r.path, drafts[r.path], reason)));
      setEditing(false);
      await load(groupPath);
    } catch {
      setActionError("저장에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function submitNewKey(groupPath: string) {
    if (!newKeyName.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      const path = groupPath ? `${groupPath}/${newKeyName.trim()}` : newKeyName.trim();
      await api.saveFile(path, newKeyValue, reason);
      setAddingKey(false);
      setNewKeyName("");
      setNewKeyValue("");
      await load(groupPath);
    } catch {
      setActionError("키 추가에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function deleteGroup() {
    if (!confirm(`/${groupPath} 아래 ${rows.length}개 키를 모두 삭제할까요?`)) return;
    setBusy(true);
    setActionError(null);
    try {
      await Promise.all(rows.map((r) => api.deleteFile(r.path)));
      onNavigate(groupPath.split("/").slice(0, -1).join("/"));
    } catch {
      setActionError("삭제에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="p-6">
      <div className="flex items-start gap-3.5 mb-1">
        <h3>{rows.length === 1 ? rows[0].name : groupPath.split("/").pop()}</h3>
        <div className="ml-auto flex gap-2">
          {!editing && (
            <button className="btn btn-secondary" disabled={!canWrite(role)} title={writeHint} onClick={startEdit}>
              편집
            </button>
          )}
          {!editing && (
            <button
              className="btn btn-secondary"
              disabled={!canWrite(role)}
              title={writeHint}
              onClick={() => setAddingKey((v) => !v)}
            >
              키 추가
            </button>
          )}
          {!editing && (
            <button className="btn btn-secondary" disabled={!canDelete(role) || busy} title={deleteHint} onClick={deleteGroup}>
              삭제
            </button>
          )}
        </div>
      </div>
      <div className="mono text-xs text-muted mb-5">/{groupPath}</div>

      {actionError && <p className="text-sm mb-4" style={{ color: "#b3432f" }}>{actionError}</p>}

      {addingKey && (
        <div className="blueprint p-5 mb-6">
          <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
          <div className="field mb-3.5">
            <label>키 이름</label>
            <input className="input mono" value={newKeyName} onChange={(e) => setNewKeyName(e.target.value)} />
          </div>
          <div className="field mb-3.5">
            <label>값</label>
            <textarea
              className="input mono"
              style={{ resize: "vertical" }}
              rows={3}
              value={newKeyValue}
              onChange={(e) => setNewKeyValue(e.target.value)}
            />
          </div>
          <div className="field mb-4">
            <label>변경 사유 (감사 로그에 기록됩니다)</label>
            <input className="input" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="예: 신규 시크릿 등록" />
          </div>
          <div className="flex gap-2.5">
            <button className="btn btn-primary" disabled={busy || !newKeyName.trim()} onClick={() => submitNewKey(groupPath)}>추가</button>
            <button className="btn btn-secondary" onClick={() => setAddingKey(false)}>취소</button>
          </div>
        </div>
      )}

      <div className="flex items-center gap-3 mb-2.5">
        <h5 className="text-[12px] tracking-[.08em] uppercase text-muted">키 {rows.length}개</h5>
        {!editing && (
          <button className="btn btn-ghost text-xs" onClick={() => setRevealed((v) => !v)}>
            {revealed ? "값 가리기" : "값 보기"}
          </button>
        )}
      </div>

      <table className="table kv">
        <thead>
          <tr>
            <th style={{ width: "26%" }}>키</th>
            <th style={{ width: editing ? "74%" : "54%" }}>값</th>
            {!editing && <th style={{ width: "20%" }}>최종 수정</th>}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.path}>
              <td className="mono font-medium">{r.name}</td>
              <td className="mono" style={{ letterSpacing: ".04em" }}>
                {editing ? (
                  <textarea
                    className="input mono"
                    style={{ minHeight: 36, resize: "vertical" }}
                    rows={Math.min(8, (drafts[r.path] ?? "").split("\n").length)}
                    value={drafts[r.path] ?? ""}
                    onChange={(e) => setDrafts((d) => ({ ...d, [r.path]: e.target.value }))}
                  />
                ) : revealed ? (
                  <pre className="mono m-0 whitespace-pre-wrap break-all">{r.content}</pre>
                ) : (
                  "•".repeat(Math.min(24, Math.max(8, r.content.length)))
                )}
              </td>
              {!editing && (
                <td className="text-muted">
                  {r.latest ? `${new Date(r.latest.timestamp).toLocaleString()} · ${r.latest.user}` : "—"}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>

      {editing && (
        <div className="blueprint p-5 mt-6">
          <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
          <div className="field mb-3.5">
            <label>변경 사유 (감사 로그에 기록됩니다)</label>
            <input className="input" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="예: 분기 회전 — INC-4821" />
          </div>
          <div className="flex items-center gap-2.5">
            <button className="btn btn-primary" disabled={busy} onClick={saveEdits}>변경 저장</button>
            <button className="btn btn-secondary" onClick={() => setEditing(false)}>취소</button>
          </div>
        </div>
      )}

      {!editing && rows.some((r) => r.latest) && (
        <>
          <h5 className="mt-8 mb-2.5 text-[12px] tracking-[.08em] uppercase text-muted">최근 변경</h5>
          <div className="text-sm">
            {rows
              .filter((r) => r.latest)
              .sort((a, b) => new Date(b.latest!.timestamp).getTime() - new Date(a.latest!.timestamp).getTime())
              .map((r) => (
                <div key={r.path} className="flex gap-4.5 py-2.5 border-t border-[color-mix(in_srgb,var(--color-text)_8%,transparent)]">
                  <span className="mono w-[132px] text-muted">{new Date(r.latest!.timestamp).toLocaleString()}</span>
                  <span className="tag tag-neutral mr-1">{r.latest!.action.toUpperCase()}</span>
                  <span>
                    <span className="mono">{r.latest!.user}</span> 님이 <span className="mono">{r.name}</span>을(를) {ACTION_LABEL[r.latest!.action] ?? r.latest!.action}
                    {r.latest!.reason ? ` — ${r.latest!.reason}` : ""}
                  </span>
                </div>
              ))}
          </div>
        </>
      )}
    </main>
  );
}
