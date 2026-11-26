import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { DemoScenario } from "../api/recsysTypes";
import type { Transport } from "../api/recsysClient";
import { readJson, removeKey, writeJson } from "../lib/storage";

const STORAGE_KEY = "lk-demo.session.v1";

export interface DemoProfile {
  id: string;
  login: string;
  group: "B2B" | "B2C";
  token: string;
}

export const DEMO_PROFILES: DemoProfile[] = [
  { id: "u1", login: "demo-b2b", group: "B2B", token: "demo-token-u1" },
  { id: "u2", login: "demo-b2c", group: "B2C", token: "demo-token-u2" },
];

interface SessionState {
  profile: DemoProfile | null;
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
  const profile = DEMO_PROFILES.find((item) => item.id === stored?.profile?.id) ?? DEMO_PROFILES[0];
  return {
    profile,
    transport: stored?.transport === "http" ? "http" : "mock",
    scenario: (stored?.scenario as DemoScenario) ?? "auto",
    generation: 0,
  };
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>(initialState);

  const loginAs = useCallback((id: string) => {
    const profile = DEMO_PROFILES.find((item) => item.id === id) ?? DEMO_PROFILES[0];
    setState((previous) => {
      const next = { ...previous, profile, generation: previous.generation + 1 };
      writeJson(STORAGE_KEY, {
        profile: next.profile,
        transport: next.transport,
        scenario: next.scenario,
      });
      return next;
    });
  }, []);

  const logout = useCallback(() => {
    removeKey(STORAGE_KEY);
    setState((previous) => ({
      ...previous,
      profile: null,
      generation: previous.generation + 1,
    }));
  }, []);

  const setTransport = useCallback((transport: Transport) => {
    setState((previous) => {
      const next = { ...previous, transport };
      writeJson(STORAGE_KEY, {
        profile: next.profile,
        transport: next.transport,
        scenario: next.scenario,
      });
      return next;
    });
  }, []);

  const setScenario = useCallback((scenario: DemoScenario) => {
    setState((previous) => {
      const next = { ...previous, scenario };
      writeJson(STORAGE_KEY, {
        profile: next.profile,
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
