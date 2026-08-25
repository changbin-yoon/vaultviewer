export type Role = "adm" | "dev" | "view";

// One LDAP group membership that grants a role scoped to a named team —
// parsed server-side from a group CN like "bi-adm" (see internal/auth's
// ResolveTeams). Purely informational: doesn't affect what `role` above
// actually permits, just lets the UI show "which team(s) grant me what".
export interface TeamGrant {
  team: string;
  role: Role;
}

export interface FileItem {
  path: string;
  name: string;
  isDir: boolean;
}

export interface VaultFile {
  path: string;
  content: string; // base64, as encoded by Go's []byte JSON marshaling
}

export interface AuditLog {
  path: string;
  action: "create" | "update" | "delete" | "rename";
  user: string;
  reason?: string;
  timestamp: string;
  previousPath?: string;
}

export interface SearchResult {
  path: string;
  snippet: string;
}

export interface Config {
  mode: string;
  backend: string;
  root: string;
  // Where the process is running (LOCAL / CLUSTER / a custom cluster name)
  // — independent of `mode`, which describes the storage backend and can
  // be "local" even inside a Kubernetes-deployed Pod.
  deployment: string;
}

export class ApiError extends Error {
  status: number;
  // Seconds to wait before retrying, from a 429 response's Retry-After
  // header (login throttling). Undefined for any other status.
  retryAfterSeconds?: number;
  constructor(status: number, message: string, retryAfterSeconds?: number) {
    super(message);
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

let token: string | null = localStorage.getItem("accesslens_token");

export function setToken(next: string | null) {
  token = next;
  if (next) localStorage.setItem("accesslens_token", next);
  else localStorage.removeItem("accesslens_token");
}

export function getToken() {
  return token;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    const text = await res.text();
    const retryAfter = res.headers.get("Retry-After");
    throw new ApiError(res.status, text || res.statusText, retryAfter ? Number(retryAfter) : undefined);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export async function login(username: string, password: string) {
  return request<{ token: string; username: string; role: Role; department: string; teams: TeamGrant[] }>(
    "/api/login",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    },
  );
}

export function me() {
  return request<{ username: string; role: Role; department: string; teams: TeamGrant[] }>("/api/me");
}

export function getConfig() {
  return request<Config>("/api/config");
}

// A connectivity check + operator-configured role/catalog labels — not a
// live Trino GRANT lookup. See internal/trino on the backend.
export interface TrinoIntegration {
  enabled: boolean;
  connected?: boolean;
  role?: string;
  catalogs?: string[];
}

export function getTrinoIntegration() {
  return request<TrinoIntegration>("/api/trino");
}

// One resolved OPA grant for the caller's mapped LDAP group — team,
// allowed operations, and catalogs come straight from OPA's live grants
// document, not AccessLens's own config. See internal/opa on the backend.
export interface OpaGrant {
  team: string;
  role: string;
  catalogs: string[];
  operations: string[];
}

export interface OpaIntegration {
  enabled: boolean;
  connected?: boolean;
  grants?: OpaGrant[];
}

export function getOpaIntegration() {
  return request<OpaIntegration>("/api/opa");
}

// A connectivity check (fixed LDAP service account) + operator-configured
// role/bucket labels — not a live bucket-policy lookup. See internal/s3iam
// on the backend.
export interface S3IamIntegration {
  enabled: boolean;
  connected?: boolean;
  role?: string;
  buckets?: string[];
}

export function getS3IamIntegration() {
  return request<S3IamIntegration>("/api/s3iam");
}

// LDAP 그룹 CN -> 화면에 보여줄 팀 이름. 역할 부여(auth.Config의
// GroupRoleMap)와는 별개로, adm이 설정 화면에서 직접 관리하는 값.
export function getGroupTeams() {
  return request<Record<string, string>>("/api/group-teams");
}

export function saveGroupTeams(mapping: Record<string, string>) {
  return request<void>("/api/group-teams", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mapping),
  });
}

export function listTree(path: string) {
  const qs = new URLSearchParams({ path });
  return request<FileItem[] | null>(`/api/tree?${qs}`);
}

export function readFile(path: string) {
  const qs = new URLSearchParams({ path });
  return request<VaultFile>(`/api/file?${qs}`);
}

export function saveFile(path: string, content: string | Blob, reason: string) {
  const qs = new URLSearchParams({ path, reason });
  return request<void>(`/api/file?${qs}`, { method: "PUT", body: content });
}

// Renames a note within the same directory (from/to's parent path must
// match — enforced server-side). Doesn't touch other notes' wikilinks —
// callers that want those kept intact rewrite and re-save them separately
// via saveFile, using markdown.ts's renameWikilinkReferences.
export function renameFile(from: string, to: string, reason: string) {
  const qs = new URLSearchParams({ from, to, reason });
  return request<void>(`/api/rename?${qs}`, { method: "PUT" });
}

export function createNamespace(path: string) {
  const qs = new URLSearchParams({ path });
  return request<void>(`/api/namespace?${qs}`, { method: "POST" });
}

export function deleteFile(path: string) {
  const qs = new URLSearchParams({ path });
  return request<void>(`/api/file?${qs}`, { method: "DELETE" });
}

export function getHistory(path: string) {
  const qs = new URLSearchParams({ path });
  return request<AuditLog[] | null>(`/api/history?${qs}`);
}

export function getAudit() {
  return request<AuditLog[] | null>("/api/audit");
}

// Browsers can't set custom headers on a WebSocket handshake, so the
// session token travels as a query param here instead of the Authorization
// header every other endpoint uses — see auditWebSocketHandler server-side.
export function auditStreamUrl(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const qs = new URLSearchParams({ token: token ?? "" });
  return `${proto}//${location.host}/ws/audit?${qs}`;
}

export function search(query: string) {
  const qs = new URLSearchParams({ q: query });
  return request<SearchResult[] | null>(`/api/search?${qs}`);
}

// The Go backend marshals []byte content fields as base64 in GET
// responses (PUT/DELETE bodies are raw bytes, not JSON, so no encoding is
// needed on the way out). Decoding via TextDecoder rather than plain atob
// keeps non-Latin1 text intact — values are frequently Korean.
export function decodeContent(base64: string): string {
  const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

const MIME_BY_EXT: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  svg: "image/svg+xml",
  webp: "image/webp",
  bmp: "image/bmp",
};

export function mimeFromExt(path: string): string {
  const ext = path.split(".").pop()?.toLowerCase() ?? "";
  return MIME_BY_EXT[ext] ?? "application/octet-stream";
}
