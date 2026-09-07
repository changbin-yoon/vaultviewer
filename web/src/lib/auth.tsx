import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import * as api from "./api";
import type { Role, TeamGrant } from "./api";

interface Session {
  username: string;
  role: Role;
  department: string;
  teams: TeamGrant[];
  // LDAP 그룹의 짧은 이름(CN). 어느 그룹이 어떤 권한을 줬는지 설명하는 데 쓴다.
  groups: string[];
}

interface AuthState {
  session: Session | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!api.getToken()) {
      setLoading(false);
      return;
    }
    api
      .me()
      .then(setSession)
      .catch(() => api.setToken(null))
      .finally(() => setLoading(false));
  }, []);

  const login = async (username: string, password: string) => {
    const res = await api.login(username, password);
    api.setToken(res.token);
    setSession({
      username: res.username,
      role: res.role,
      department: res.department,
      teams: res.teams,
      groups: res.groups ?? [],
    });
  };

  const logout = () => {
    api.setToken(null);
    setSession(null);
  };

  return <AuthContext.Provider value={{ session, loading, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function canWrite(role: Role) {
  return role === "adm" || role === "dev";
}

export function canDelete(role: Role) {
  return role === "adm";
}
