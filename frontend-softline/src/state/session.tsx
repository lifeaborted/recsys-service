import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { login, me, type Transport } from "../api/recsysClient";
import type { MeResponse } from "../api/recsysTypes";

export const RECSYS_API = import.meta.env.VITE_RECSYS_API_BASE ?? "/api/v1";
const TOKEN_KEY = "recsys-preview.session";

export type SessionProfile = MeResponse & { token: string };

export function profileStatus(profile: MeResponse) {
  if (profile.role === "admin") return "Администратор";
  if (profile.role === "verified") return `Верифицированный ${profile.client_group}`;
  return "Неверифицированный аккаунт";
}

interface SessionState {
  profile: SessionProfile | null;
  loading: boolean;
  error: string | null;
  transport: Transport;
  signIn: (name: string, password: string) => Promise<void>;
  logout: () => void;
}

const SessionContext = createContext<SessionState | null>(null);
function config(token: string) {
  return { baseUrl: RECSYS_API, transport: "http" as const, token };
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [profile, setProfile] = useState<SessionProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const attempt = useRef(0);

  useEffect(() => {
    const token = sessionStorage.getItem(TOKEN_KEY);
    if (!token) {
      setLoading(false);
      return;
    }
    const revision = ++attempt.current;
    void me(config(token)).then((data) => {
      if (revision === attempt.current) setProfile({ ...data, token });
    }).catch(() => {
      if (revision === attempt.current) sessionStorage.removeItem(TOKEN_KEY);
    }).finally(() => {
      if (revision === attempt.current) setLoading(false);
    });
    return () => { attempt.current += 1; };
  }, []);

  const signIn = useCallback(async (name: string, password: string) => {
    const revision = ++attempt.current;
    setError(null);
    setLoading(true);
    try {
      const { access_token: token } = await login(config(""), name, password);
      const data = await me(config(token));
      if (revision === attempt.current) {
        sessionStorage.setItem(TOKEN_KEY, token);
        setProfile({ ...data, token });
      }
    } catch (cause) {
      if (revision === attempt.current) setError(cause instanceof Error ? cause.message : "Ошибка входа");
    } finally {
      if (revision === attempt.current) setLoading(false);
    }
  }, []);

  const logout = useCallback(() => {
    attempt.current += 1;
    sessionStorage.removeItem(TOKEN_KEY);
    setProfile(null);
    setLoading(false);
    setError(null);
  }, []);

  return <SessionContext.Provider value={{ profile, loading, error, transport: "http", signIn, logout }}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionState {
  const context = useContext(SessionContext);
  if (!context) throw new Error("SessionProvider is missing");
  return context;
}
