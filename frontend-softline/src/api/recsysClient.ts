/**
 * Клиент рекомендаций по api/openapi.yaml. Один и тот же интерфейс работает
 * на синтетических примерах (подмена транспорта) и на реальном API /api/v1.
 */
import emptyExample from "./examples/recommendations.empty.json";
import errorExample from "./examples/recommendations.error.json";
import fullExample from "./examples/recommendations.full.json";
import partialExample from "./examples/recommendations.partial.json";
import verifiedExample from "./examples/recommendations.verified.json";
import type {
  DemoScenario,
  MeResponse,
  Recommendations,
  SessionResponse,
} from "./recsysTypes";

export type Transport = "mock" | "http";

export interface RecsysClientConfig {
  baseUrl: string;
  transport: Transport;
  token: string;
}

export class ApiError extends Error {
  readonly requestId: string;
  readonly code: string;
  readonly status: number;

  constructor(message: string, requestId: string, code: string, status = 0) {
    super(message);
    this.name = "ApiError";
    this.requestId = requestId;
    this.code = code;
    this.status = status;
  }
}

const examples: Record<string, Recommendations> = {
  full: fullExample as Recommendations,
  partial: partialExample as Recommendations,
  empty: emptyExample as Recommendations,
  check: verifiedExample as Recommendations,
};

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("Aborted", "AbortError"));
      return;
    }
    const timer = window.setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        window.clearTimeout(timer);
        reject(new DOMException("Aborted", "AbortError"));
      },
      { once: true },
    );
  });
}

function scenarioFor(scenario: DemoScenario, profileId: string): string {
  if (scenario !== "auto") return scenario;
  if (profileId === "u3") return "check";
  return profileId === "u2" ? "partial" : "full";
}

async function request<T>(cfg: RecsysClientConfig, path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${cfg.baseUrl}${path}`, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...(cfg.token ? { Authorization: `Bearer ${cfg.token}` } : {}),
    },
  });
  const payload = await response.json().catch(() => null) as Record<string, unknown> | null;
  if (!response.ok) {
    const error = payload?.error as Record<string, string> | undefined;
    throw new ApiError(
      error?.message ?? `HTTP ${response.status}`,
      typeof payload?.request_id === "string" ? payload.request_id : "unknown",
      error?.code ?? "request_failed",
      response.status,
    );
  }
  return payload as T;
}

export async function getRecommendations(
  cfg: RecsysClientConfig,
  options: { scenario: DemoScenario; profileId: string; limit?: number; signal?: AbortSignal },
): Promise<Recommendations> {
  const scenario = scenarioFor(options.scenario, options.profileId);
  const limit = options.limit ?? 3;

  if (cfg.transport === "mock") {
    if (scenario === "slow") await delay(1500, options.signal);
    if (scenario === "error") {
      const example = errorExample as unknown as {
        request_id: string;
        error: { code: string; message: string };
      };
      throw new ApiError(example.error.message, example.request_id, example.error.code, 500);
    }
    const example = examples[scenario] ?? examples.full;
    const result = structuredClone(example);
    result.items = result.items.slice(0, limit);
    return result;
  }

  return request<Recommendations>(
    cfg,
    `/recommendations/${cfg.token ? "me" : "guest"}?scenario=${encodeURIComponent(scenario)}&limit=${limit}`,
    { signal: options.signal },
  );
}

export async function login(
  cfg: RecsysClientConfig,
  loginName: string,
  password: string,
): Promise<SessionResponse> {
  if (cfg.transport === "mock") {
    const token = loginName === "demo-b2c" ? "demo-token-u2" : "demo-token-u1";
    return { access_token: token, expires_at: new Date(Date.now() + 86_400_000).toISOString() };
  }
  return request<SessionResponse>(cfg, "/auth/login", {
    method: "POST",
    body: JSON.stringify({ login: loginName, password }),
  });
}

export async function me(cfg: RecsysClientConfig): Promise<MeResponse> {
  if (cfg.transport === "mock") {
    return cfg.token === "demo-token-u2"
      ? { profile_id: "u2", client_id: "DEMO-B2C", client_group: "B2C", login: "demo-b2c", role: "verified" }
      : { profile_id: "u1", client_id: "DEMO-B2B", client_group: "B2B", login: "demo-b2b", role: "verified" };
  }
  return request<MeResponse>(cfg, "/users/me");
}

export interface CatalogProduct {
  item_id: string;
  title: string;
  available: boolean;
}

export interface CatalogResponse {
  items: CatalogProduct[];
  catalog_version?: string;
  feed_generated_at?: string;
}

export function listProducts(cfg: RecsysClientConfig, signal?: AbortSignal): Promise<CatalogResponse> {
  return request(cfg, "/products?limit=100", { signal });
}

export async function purchase(cfg: RecsysClientConfig, itemId: string): Promise<void> {
  if (cfg.transport === "mock") return;
  await request(cfg, "/purchases", { method: "POST", body: JSON.stringify({ item_id: itemId }) });
}

export interface PurchaseHistoryItem {
  order_id: string;
  title: string;
  purchased_at: string;
  total?: number;
  currency?: string;
}

export function listPurchases(cfg: RecsysClientConfig, signal?: AbortSignal): Promise<{ items: PurchaseHistoryItem[] }> {
  return request(cfg, "/purchases", { signal });
}

export type DebugMode = "clean" | "latest";
export type DebugTarget = "guest" | "notver" | "verb2b" | "verb2c" | "admin";

export interface RecommendationDiagnostics {
  target: DebugTarget;
  mode: DebugMode;
  session?: { started_at: string; purchase_count: number };
  duration_ms: number;
  recommendations: Recommendations;
  tables: Record<string, number>;
  runs: unknown[];
}

export function getRecommendationDiagnostics(cfg: RecsysClientConfig, target: DebugTarget, mode: DebugMode, signal?: AbortSignal) {
  return request<RecommendationDiagnostics>(cfg, `/admin/recommendations?target=${encodeURIComponent(target)}&limit=36&mode=${mode}`, { signal });
}
