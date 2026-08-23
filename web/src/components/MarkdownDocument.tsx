import { useEffect, useRef, useState, Children, isValidElement, type ReactNode } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import * as api from "../lib/api";
import type { AuditLog, FileItem } from "../lib/api";
import { splitFrontmatter, preprocessObsidian, resolveLinkTarget, stripMdExtension } from "../lib/markdown";
import { useAuth, canWrite, canDelete } from "../lib/auth";
import { getVaultIndex } from "../lib/vaultIndex";
import { MermaidDiagram } from "./MermaidDiagram";

// react-markdown's default urlTransform strips any URL scheme it doesn't
// recognize (an XSS precaution) — which silently empties our internal
// "wikilink:"/"embed:" pseudo-schemes. Allow just those two through
// unchanged and defer to the default sanitizer for everything else.
function urlTransform(url: string): string {
  if (url.startsWith("wikilink:") || url.startsWith("embed:")) return url;
  return defaultUrlTransform(url);
}

function parentDir(path: string) {
  const parts = path.split("/");
  parts.pop();
  return parts.join("/");
}

function childText(node: ReactNode): string {
  if (typeof node === "string") return node;
  if (typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(childText).join("");
  if (isValidElement(node)) return childText((node.props as { children?: ReactNode }).children);
  return "";
}

const CALLOUT_LABEL: Record<string, string> = {
  info: "INFO",
  note: "NOTE",
  tip: "TIP",
  hint: "TIP",
  warning: "WARNING",
  caution: "WARNING",
  danger: "DANGER",
  error: "DANGER",
  success: "SUCCESS",
  done: "SUCCESS",
  check: "SUCCESS",
  question: "QUESTION",
  help: "QUESTION",
  bug: "BUG",
  example: "EXAMPLE",
  quote: "QUOTE",
  abstract: "SUMMARY",
  summary: "SUMMARY",
  todo: "TODO",
};

function Callout({ type, title, children }: { type: string; title: string; children: ReactNode }) {
  const label = CALLOUT_LABEL[type.toLowerCase()] ?? type.toUpperCase();
  return (
    <div className="callout">
      <div className="flex items-center gap-2 mb-1.5">
        <span className="tag tag-accent mono">{label}</span>
        {title && <strong>{title}</strong>}
      </div>
      <div className="callout-body">{children}</div>
    </div>
  );
}

function Blockquote({ children }: { children?: ReactNode }) {
  // Children.toArray() includes the whitespace text nodes remark leaves
  // between block-level children (e.g. the newline between two
  // paragraphs) as their own array entries — drop those so items[0] is
  // actually the first paragraph, not a stray "\n".
  const items = Children.toArray(children).filter(
    (item) => !(typeof item === "string" && item.trim() === "")
  );
  const first = items[0];
  const m = childText(first).trim().match(/^%%CALLOUT:(\w+):(.*)%%$/);
  if (m) {
    return (
      <Callout type={m[1]} title={m[2]}>
        {items.slice(1)}
      </Callout>
    );
  }
  return <blockquote className="md-quote">{children}</blockquote>;
}

function EmbedImage({ target, currentDir, alt }: { target: string; currentDir: string; alt: string }) {
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const candidates = [
      resolveLinkTarget(target, currentDir, true),
      `attachments/${decodeURIComponent(target)}`,
    ];
    (async () => {
      for (const path of candidates) {
        try {
          const file = await api.readFile(path);
          if (!cancelled) setSrc(`data:${api.mimeFromExt(path)};base64,${file.content}`);
          return;
        } catch {
          continue;
        }
      }
      if (!cancelled) setFailed(true);
    })();
    return () => {
      cancelled = true;
    };
  }, [target, currentDir]);

  if (failed) return <span className="tag tag-outline">첨부 파일을 찾을 수 없음: {alt}</span>;
  if (!src) return <span className="text-muted text-sm">이미지 불러오는 중…</span>;
  return <img src={src} alt={alt} className="max-w-full" />;
}

// Resolves any image reference that isn't already an absolute URL through
// the vault storage engine — this covers both Obsidian embeds
// (![[file]], pre-rewritten to an "embed:" pseudo-src) and plain Markdown
// images (![alt](file.png)), which Obsidian itself also produces (e.g. on
// pasting a screenshot) and which a bare <img src> can't resolve since
// there's no static file route for arbitrary vault paths.
function MarkdownImage({ src, alt, currentDir }: { src: string; alt: string; currentDir: string }) {
  if (/^(https?:|data:)/.test(src)) {
    return <img src={src} alt={alt} className="max-w-full" />;
  }
  const target = src.startsWith("embed:") ? src.slice("embed:".length) : encodeURIComponent(src);
  return <EmbedImage target={target} currentDir={currentDir} alt={alt} />;
}

function Link({
  href,
  children,
  currentDir,
  onNavigate,
}: {
  href?: string;
  children?: ReactNode;
  currentDir: string;
  onNavigate: (path: string) => void;
}) {
  if (href?.startsWith("wikilink:")) {
    const target = href.slice("wikilink:".length);
    return (
      <button
        className="wikilink"
        onClick={() => onNavigate(resolveLinkTarget(target, currentDir, false))}
      >
        {children}
      </button>
    );
  }
  return (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  );
}

interface Props {
  path: string;
  onNavigate: (path: string) => void;
  // Called after this note is deleted, so the sidebar tree (which doesn't
  // share state with this component) knows to refresh.
  onMutate?: () => void;
}

export function MarkdownDocument({ path, onNavigate, onMutate }: Props) {
  const { session } = useAuth();
  const role = session!.role;
  const [content, setContent] = useState<string | null>(null);
  const [siblings, setSiblings] = useState<FileItem[]>([]);
  const [latest, setLatest] = useState<AuditLog | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [backlinks, setBacklinks] = useState<string[] | null>(null);
  const [uploading, setUploading] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const dir = parentDir(path);
  const name = path.split("/").pop() ?? path;

  useEffect(() => {
    setEditing(false);
    setConfirmingDelete(false);
    setError(null);
    setContent(null);
    api
      .readFile(path)
      .then((file) => {
        const text = api.decodeContent(file.content);
        setContent(text);
        // A freshly created, still-empty note has nothing to view — open
        // it straight into the editor instead of an empty preview.
        if (text === "" && canWrite(role)) {
          setDraft("");
          setReason("");
          setEditing(true);
        }
      })
      .catch(() => setError("문서를 찾을 수 없습니다."));
    api
      .getHistory(path)
      .then((h) => setLatest(h && h.length > 0 ? h[h.length - 1] : null))
      .catch(() => setLatest(null));
    api
      .listTree(dir)
      .then((items) => setSiblings((items ?? []).filter((i) => !i.isDir && i.name.endsWith(".md"))))
      .catch(() => setSiblings([]));
    setBacklinks(null);
    getVaultIndex()
      .then((idx) => setBacklinks(idx.backlinks.get(path) ?? []))
      .catch(() => setBacklinks([]));
  }, [path, dir]);

  async function save() {
    setBusy(true);
    try {
      await api.saveFile(path, draft, reason);
      setContent(draft);
      setEditing(false);
      void getVaultIndex(true);
    } catch {
      setError("저장에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  // Uploads an image into the vault's shared attachments/ folder (never
  // next to arbitrary notes) and inserts an Obsidian embed reference at the
  // textarea's cursor, so images stay organized in one place regardless of
  // which note they're added from.
  async function uploadImage(file: File) {
    setUploading(true);
    setError(null);
    try {
      const dot = file.name.lastIndexOf(".");
      const base = dot > 0 ? file.name.slice(0, dot) : file.name;
      const ext = dot > 0 ? file.name.slice(dot) : "";
      const storedName = `${base}_${Math.random().toString(16).slice(2, 8)}${ext}`;
      await api.saveFile(`attachments/${storedName}`, file, `이미지 업로드: ${file.name}`);
      void getVaultIndex(true);

      const embed = `![[${storedName}]]`;
      const el = textareaRef.current;
      if (el) {
        const start = el.selectionStart ?? draft.length;
        const end = el.selectionEnd ?? draft.length;
        const next = draft.slice(0, start) + embed + draft.slice(end);
        setDraft(next);
        requestAnimationFrame(() => {
          el.focus();
          el.selectionStart = el.selectionEnd = start + embed.length;
        });
      } else {
        setDraft((d) => d + embed);
      }
    } catch {
      setError("이미지 업로드에 실패했습니다.");
    } finally {
      setUploading(false);
    }
  }

  async function remove() {
    setConfirmingDelete(false);
    setBusy(true);
    try {
      await api.deleteFile(path);
      void getVaultIndex(true);
      onMutate?.();
      onNavigate(dir);
    } catch {
      setError("삭제에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  const writeHint = !canWrite(role) ? "view 역할은 수정할 수 없습니다" : "";
  const deleteHint = !canDelete(role) ? "삭제는 adm 역할만 가능합니다" : "";

  const { frontmatter, body } = content != null ? splitFrontmatter(content) : { frontmatter: null, body: "" };

  return (
    <main className="p-6 grid" style={{ gridTemplateColumns: siblings.length > 1 ? "200px 1fr" : "1fr", gap: 28 }}>
      {siblings.length > 1 && (
        <nav className="text-[13px]">
          <div className="text-[10px] tracking-[.12em] uppercase text-muted mb-2">이 폴더의 문서</div>
          <div className="flex flex-col">
            {siblings.map((s) => (
              <button
                key={s.path}
                className={`tree-row !px-2${s.path === path ? " selected" : ""}`}
                onClick={() => onNavigate(s.path)}
              >
                {stripMdExtension(s.name)}
              </button>
            ))}
          </div>
        </nav>
      )}

      <div className="min-w-0">
        <div className="flex items-start gap-3.5 mb-1">
          <h3>{stripMdExtension(name)}</h3>
          <div className="ml-auto flex gap-2">
            {!editing && (
              <button
                className="btn btn-secondary"
                disabled={!canWrite(role) || content == null}
                title={writeHint}
                onClick={() => {
                  setDraft(content ?? "");
                  setReason("");
                  setEditing(true);
                }}
              >
                편집
              </button>
            )}
            {!editing && confirmingDelete && (
              <div className="flex items-center gap-2">
                <span className="text-sm" style={{ color: "#b3432f" }}>
                  {stripMdExtension(name)}을(를) 삭제할까요?
                </span>
                <button className="btn btn-secondary" disabled={busy} onClick={remove}>
                  삭제 확인
                </button>
                <button className="btn btn-secondary" disabled={busy} onClick={() => setConfirmingDelete(false)}>
                  취소
                </button>
              </div>
            )}
            {!editing && !confirmingDelete && (
              <button
                className="btn btn-secondary"
                disabled={!canDelete(role) || busy}
                title={deleteHint}
                onClick={() => setConfirmingDelete(true)}
              >
                삭제
              </button>
            )}
          </div>
        </div>
        <div className="mono text-xs text-muted mb-5">
          /{stripMdExtension(path)}
          {latest && ` · ${new Date(latest.timestamp).toLocaleString()} · ${latest.user}`}
        </div>

        {error && <p className="text-sm mb-4" style={{ color: "#b3432f" }}>{error}</p>}

        {content == null && !error && <p className="text-sm text-muted">불러오는 중…</p>}

        {editing ? (
          <>
            <div className="flex justify-end mb-2">
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) void uploadImage(file);
                  e.target.value = "";
                }}
              />
              <button
                className="btn btn-secondary"
                disabled={uploading}
                onClick={() => fileInputRef.current?.click()}
              >
                {uploading ? "업로드 중…" : "이미지 추가"}
              </button>
            </div>
            <textarea
              ref={textareaRef}
              className="input mono"
              style={{ minHeight: 420, resize: "vertical" }}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
            />
            <div className="blueprint p-5 mt-5">
              <i className="corner tl" /><i className="corner tr" /><i className="corner bl" /><i className="corner br" />
              <div className="field mb-3.5">
                <label>변경 사유 (감사 로그에 기록됩니다)</label>
                <input className="input" value={reason} onChange={(e) => setReason(e.target.value)} />
              </div>
              <div className="flex gap-2.5">
                <button className="btn btn-primary" disabled={busy} onClick={save}>변경 저장</button>
                <button className="btn btn-secondary" onClick={() => setEditing(false)}>취소</button>
              </div>
            </div>
          </>
        ) : (
          content != null && (
            <article className="md-body">
              {frontmatter && (
                <pre className="md-frontmatter mono">{frontmatter}</pre>
              )}
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                urlTransform={urlTransform}
                components={{
                  blockquote: Blockquote,
                  a: (p) => <Link href={p.href} currentDir={dir} onNavigate={onNavigate}>{p.children}</Link>,
                  img: (p) => <MarkdownImage src={p.src ?? ""} alt={p.alt ?? ""} currentDir={dir} />,
                  code: (p) => {
                    const lang = /language-(\w+)/.exec(p.className ?? "")?.[1];
                    if (lang === "mermaid") {
                      return <MermaidDiagram code={childText(p.children)} />;
                    }
                    return <code className={p.className}>{p.children}</code>;
                  },
                }}
              >
                {preprocessObsidian(body)}
              </ReactMarkdown>
            </article>
          )
        )}

        {!editing && content != null && (
          <div className="mt-8 pt-5 border-t border-[var(--color-divider)]">
            <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">
              백링크 {backlinks != null && `(${backlinks.length})`}
            </h5>
            {backlinks == null ? (
              <p className="text-sm text-muted">불러오는 중…</p>
            ) : backlinks.length === 0 ? (
              <p className="text-sm text-muted">이 문서를 링크하는 다른 노트가 없습니다.</p>
            ) : (
              <div className="flex flex-col gap-1">
                {backlinks.map((p) => (
                  <button key={p} className="wikilink text-left text-sm" onClick={() => onNavigate(p)}>
                    {stripMdExtension(p)}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </main>
  );
}
