import { useState, type FormEvent } from "react";
import { useAuth } from "../lib/auth";
import { ApiError } from "../lib/api";

export function LoginPage() {
  const { login } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      await login(username, password);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError("아이디 또는 비밀번호가 올바르지 않거나, 할당된 역할이 없습니다.");
      } else if (err instanceof ApiError && err.status === 429) {
        const wait = err.retryAfterSeconds;
        setError(`로그인 시도가 너무 잦습니다. ${wait ? `${wait}초 후` : "잠시 후"} 다시 시도해주세요.`);
      } else {
        setError("로그인 중 오류가 발생했습니다. 잠시 후 다시 시도해주세요.");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-6">
      <div className="w-full max-w-[420px] flex flex-col gap-10">
        <div className="font-[var(--font-heading)] font-semibold text-[22px] tracking-wide">
          VAULT<span className="text-[var(--color-accent)]">VIEWER</span>
        </div>
        <form onSubmit={onSubmit} className="blueprint p-8">
          <i className="corner tl" />
          <i className="corner tr" />
          <i className="corner bl" />
          <i className="corner br" />
          <h3 className="mb-1.5">LDAP 로그인</h3>
          <p className="text-sm text-muted mb-6">디렉터리 그룹에 따라 역할이 자동으로 결정됩니다.</p>

          <div className="field mb-3.5">
            <label>사용자 ID</label>
            <input
              className="input mono"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoFocus
            />
          </div>
          <div className="field mb-4.5">
            <label>비밀번호</label>
            <input
              className="input"
              type="password"
              placeholder="••••••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          {error && <p className="text-sm mb-3" style={{ color: "#b3432f" }}>{error}</p>}

          <button className="btn btn-primary btn-block" disabled={busy || !username || !password}>
            {busy ? "로그인 중…" : "로그인"}
          </button>
        </form>
      </div>
    </div>
  );
}
