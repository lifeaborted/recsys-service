import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { Scenario } from "../api/types";
import { readJson, removeKey, writeJson } from "../lib/storage";

const STORAGE_KEY = "recsys-portal.settings.v1";

export interface Settings {
  userId: string;
  baseUrl: string;
  token: string;
  mockMode: boolean;
  scenario: Scenario;
  limit: number;
  timeoutMs: number;
}

export const DEFAULT_SETTINGS: Settings = {
  userId: "u1",
  baseUrl: import.meta.env.VITE_RECSYS_BASE_URL ?? "/api",
  token: "",
  mockMode: true,
  scenario: "demo",
  limit: 9,
  timeoutMs: 8000,
};

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function loadSettings(): Settings {
  const stored = readJson<Partial<Settings>>(STORAGE_KEY);
  if (!stored) return DEFAULT_SETTINGS;

  const scenario: Scenario =
    stored.scenario === "demo" ||
    stored.scenario === "upsell" ||
    stored.scenario === "cross-sell" ||
    stored.scenario === "onboarding" ||
    stored.scenario === "retention"
      ? stored.scenario
      : DEFAULT_SETTINGS.scenario;

  return {
    ...DEFAULT_SETTINGS,
    ...stored,
    scenario,
    userId: typeof stored.userId === "string" ? stored.userId : DEFAULT_SETTINGS.userId,
    baseUrl:
      typeof stored.baseUrl === "string" && stored.baseUrl.trim()
        ? stored.baseUrl
        : DEFAULT_SETTINGS.baseUrl,
    limit: clamp(Number(stored.limit) || DEFAULT_SETTINGS.limit, 1, 50),
    timeoutMs: clamp(Number(stored.timeoutMs) || DEFAULT_SETTINGS.timeoutMs, 500, 60000),
  };
}

interface SettingsContextValue {
  settings: Settings;
  update: (patch: Partial<Settings>) => void;
  reset: () => void;
}

const SettingsContext = createContext<SettingsContextValue | null>(null);

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<Settings>(loadSettings);

  const update = useCallback((patch: Partial<Settings>) => {
    setSettings((previous) => {
      const next = { ...previous, ...patch };
      writeJson(STORAGE_KEY, next);
      return next;
    });
  }, []);

  const reset = useCallback(() => {
    removeKey(STORAGE_KEY);
    setSettings(DEFAULT_SETTINGS);
  }, []);

  const value = useMemo(() => ({ settings, update, reset }), [settings, update, reset]);

  return <SettingsContext.Provider value={value}>{children}</SettingsContext.Provider>;
}

export function useSettings(): SettingsContextValue {
  const context = useContext(SettingsContext);
  if (!context) {
    throw new Error("useSettings должен вызываться внутри SettingsProvider");
  }
  return context;
}
