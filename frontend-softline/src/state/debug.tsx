import {
  createContext,
  useContext,
  useMemo,
  useSyncExternalStore,
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

interface StoreState {
  snapshot: DebugSnapshot | null;
  history: DebugSnapshot[];
  showScores: boolean;
  rerun: (() => void) | null;
}

// Внешний стор: обновления не вызывают setState внутри рендера компонента.
let store: StoreState = { snapshot: null, history: [], showScores: false, rerun: null };
const listeners = new Set<() => void>();

function publish(patch: Partial<StoreState>): void {
  store = { ...store, ...patch };
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot(): StoreState {
  return store;
}

interface DebugActions {
  update: (snapshot: DebugSnapshot) => void;
  setRerun: (fn: (() => void) | null) => void;
  toggleScores: () => void;
  clearHistory: () => void;
}

const DebugContext = createContext<DebugActions | null>(null);

const actions: DebugActions = {
  update: (snapshot) =>
    publish({ snapshot, history: [snapshot, ...store.history].slice(0, 20) }),
  setRerun: (rerun) => publish({ rerun }),
  toggleScores: () => publish({ showScores: !store.showScores }),
  clearHistory: () => publish({ history: [] }),
};

export function DebugProvider({ children }: { children: ReactNode }) {
  const value = useMemo(() => actions, []);
  return <DebugContext.Provider value={value}>{children}</DebugContext.Provider>;
}

export function useDebug() {
  const context = useContext(DebugContext);
  if (!context) throw new Error("useDebug должен вызываться внутри DebugProvider");
  const state = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return { ...state, ...context };
}

// Экспорт для проверок вне React (не используется UI).
export const debugStore = {
  get: getSnapshot,
  reset: () => publish({ snapshot: null, history: [], showScores: false, rerun: null }),
};
