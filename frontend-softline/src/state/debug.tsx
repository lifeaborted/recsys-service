import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { RecommendationItem, RecommendationResponse } from "../api/types";
import type { RecommendationsMeta } from "../hooks/useRecommendations";

export interface DebugSnapshot {
  request: unknown;
  response: RecommendationResponse | null;
  error: string | null;
  meta: RecommendationsMeta | null;
  items: RecommendationItem[];
}

interface DebugContextValue {
  snapshot: DebugSnapshot | null;
  history: DebugSnapshot[];
  showScores: boolean;
  rerun: (() => void) | null;
  update: (snapshot: DebugSnapshot) => void;
  setRerun: (fn: (() => void) | null) => void;
  toggleScores: () => void;
  clearHistory: () => void;
}

const DebugContext = createContext<DebugContextValue | null>(null);

export function DebugProvider({ children }: { children: ReactNode }) {
  const [snapshot, setSnapshot] = useState<DebugSnapshot | null>(null);
  const [history, setHistory] = useState<DebugSnapshot[]>([]);
  const [showScores, setShowScores] = useState(false);
  const [rerun, setRerun] = useState<(() => void) | null>(null);

  const update = useCallback((next: DebugSnapshot) => {
    // Откладываем запись в стор, чтобы обновление провайдера никогда не
    // попадало в фазу рендера дочернего компонента (React 18 StrictMode).
    queueMicrotask(() => {
      setSnapshot(next);
      setHistory((previous) => [next, ...previous].slice(0, 20));
    });
  }, []);

  const toggleScores = useCallback(() => setShowScores((value) => !value), []);
  const clearHistory = useCallback(() => setHistory([]), []);

  const value = useMemo(
    () => ({ snapshot, history, showScores, rerun, update, setRerun, toggleScores, clearHistory }),
    [snapshot, history, showScores, rerun, update, toggleScores, clearHistory],
  );

  return <DebugContext.Provider value={value}>{children}</DebugContext.Provider>;
}

export function useDebug(): DebugContextValue {
  const context = useContext(DebugContext);
  if (!context) throw new Error("useDebug должен вызываться внутри DebugProvider");
  return context;
}
