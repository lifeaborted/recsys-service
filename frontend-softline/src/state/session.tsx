import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  useRef,
  useEffect,
  type ReactNode,
} from "react";
import type { DemoScenario } from "../api/recsysTypes";
import { login, type Transport } from "../api/recsysClient";
import { readJson, removeKey, writeJson } from "../lib/storage";

const STORAGE_KEY = "lk-demo.session.v2";

export interface DemoProfile {
  id: string;
  login: string;
  group: "B2B" | "B2C";
  token: string;
  /** Синтетический профиль без истории: доступен только на mock-транспорте. */
  mockOnly?: boolean;
}

export const DEMO_PROFILES: DemoProfile[] = [
  { id: "u1", login: "demo-b2b", group: "B2B", token: "demo-token-u1" },
  { id: "u2", login: "demo-b2c", group: "B2C", token: "demo-token-u2" },
  { id: "u3", login: "demo-cold", group: "B2C", token: "demo-token-u3", mockOnly: true },
];

interface SessionState {
  profile: DemoProfile | null;
  preferredId: string;
  error?: string;
  transport: Transport;
  scenario: DemoScenario;
  // Меняется при смене профиля/выходе: старые ответы должны игнорироваться.
  generation: number;
}

interface SessionContextValue extends SessionState {
  loginAs: (id: string) => void;
  logout: () => void;
  setTransport: (transport: Transport) => void;
  setScenario: (scenario: DemoScenario) => void;
}

const SessionContext = createContext<SessionContextValue | null>(null);

function initialState(): SessionState {
  const stored = readJson<Partial<SessionState>>(STORAGE_KEY);
  const preferredId = stored?.profile?.id ?? "u1";
  const transport: Transport = stored?.transport === "mock" ? "mock" : "http";
  return {
    profile: transport === "mock"
      ? (DEMO_PROFILES.find((item) => item.id === preferredId) ?? DEMO_PROFILES[0])
      : null,
    preferredId,
    transport,
    scenario: (stored?.scenario as DemoScenario) ?? "auto",
    generation: 0,
  };
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>(initialState);

  const attempt = useRef(0);
  const signIn = useCallback(async (id: string, transport: Transport) => {
    const current = ++attempt.current;
    const profile = DEMO_PROFILES.find((item) => item.id === id) ?? DEMO_PROFILES[0];
    if (transport === "http" && profile.mockOnly) {
      setState((prev) => ({
        ...prev,
        error: "Профиль без истории доступен только на синтетическом транспорте.",
      }));
      return;
    }
    setState((prev) => ({ ...prev, transport, profile: null, error: undefined, generation: prev.generation + 1 }));
    let signed = profile;
    if (transport === "http") {
      try {
        const password = profile.group === "B2C"
          ? (import.meta.env.VITE_DEMO_B2C_PASSWORD ?? "demo-b2c-pass")
          : (import.meta.env.VITE_DEMO_B2B_PASSWORD ?? "demo-b2b-pass");
        const session = await login({ baseUrl: import.meta.env.VITE_RECSYS_API_BASE ?? "/api/v1", transport, token: "" }, profile.login, password);
        signed = { ...profile, token: session.access_token };
      } catch (error) {
        if (attempt.current === current) setState((prev) => ({ ...prev, error: error instanceof Error ? error.message : "Ошибка входа" }));
        return;
      }
    }
    if (attempt.current !== current) return;
    setState((previous) => {
      const next = { ...previous, transport, profile: signed, generation: previous.generation + 1 };
      writeJson(STORAGE_KEY, {
        profile: next.profile ? { id: next.profile.id } : null,
        transport: next.transport,
        scenario: next.scenario,
      });
      return next;
    });
  }, []);

  // По умолчанию экран работает с реальным API: логинимся сразу при загрузке.
  useEffect(() => {
    if (state.transport === "http" && !state.profile && !state.error) {
      void signIn(state.preferredId, "http");
    }
  }, [state.transport, state.profile, state.error, state.preferredId, signIn]);

  const loginAs = useCallback((id: string) => {
    const profile = DEMO_PROFILES.find((item) => item.id === id);
    // Профиль без истории живёт только в синтетическом примере, поэтому включает mock.
    const transport: Transport = profile?.mockOnly ? "mock" : state.transport;
    void signIn(id, transport);
  }, [signIn, state.transport]);

  const logout = useCallback(() => {
    attempt.current++;
    removeKey(STORAGE_KEY);
    setState((previous) => ({
      ...previous,
      profile: null,
      error: undefined,
      generation: previous.generation + 1,
    }));
  }, []);

  const setTransport = useCallback((transport: Transport) => {
    const current = DEMO_PROFILES.find((item) => item.id === state.profile?.id);
    const nextId = transport === "http" && current?.mockOnly ? "u1" : (state.profile?.id ?? "u1");
    void signIn(nextId, transport);
  }, [signIn, state.profile?.id]);

  const setScenario = useCallback((scenario: DemoScenario) => {
    setState((previous) => {
      const next = { ...previous, scenario };
      writeJson(STORAGE_KEY, {
        profile: next.profile ? { id: next.profile.id } : null,
        transport: next.transport,
        scenario: next.scenario,
      });
      return next;
    });
  }, []);

  const value = useMemo(
    () => ({ ...state, loginAs, logout, setTransport, setScenario }),
    [state, loginAs, logout, setTransport, setScenario],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const context = useContext(SessionContext);
  if (!context) throw new Error("useSession должен вызываться внутри SessionProvider");
  return context;
}
