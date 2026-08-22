export type Role = "adm" | "dev" | "view";

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
  action: "create" | "update" | "delete";
  user: string;
  reason?: string;
  timestamp: string;
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
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

let token: string | null = localStorage.getItem("vaultviewer_token");

export function setToken(next: string | null) {
  token = next;
  if (next) localStorage.setItem("vaultviewer_token", next);
  else localStorage.removeItem("vaultviewer_token");
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
    throw new ApiError(res.status, text || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export async function login(username: string, password: string) {
  return request<{ token: string; username: string; role: Role }>("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
}

export function me() {
  return request<{ username: string; role: Role }>("/api/me");
}

export function getConfig() {
  return request<Config>("/api/config");
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
