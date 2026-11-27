import type {
  FeedbackPayload,
  FeedbackResponse,
  HealthResponse,
  ProductDetail,
  RecommendationItem,
  ScoreHistory,
  RecommendationResponse,
} from "./types";

export interface RecsysConfig {
  baseUrl: string;
  token?: string;
  timeoutMs?: number;
}

export class RecsysError extends Error {
  readonly status: number | null;
  readonly details: unknown;

  constructor(message: string, status: number | null = null, details: unknown = null) {
    super(message);
    this.name = "RecsysError";
    this.status = status;
    this.details = details;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}

function textValue(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function asNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() && Number.isFinite(Number(value))) {
    return Number(value);
  }
  return undefined;
}

function normalizeItem(raw: unknown, index: number): RecommendationItem {
  if (!isRecord(raw)) {
    throw new RecsysError(`Элемент items[${index}] не является объектом`);
  }

  const tags = Array.isArray(raw.tags)
    ? raw.tags.filter((tag): tag is string => typeof tag === "string")
    : undefined;

  return {
    item_id: asString(raw.item_id) ?? asString(raw.id) ?? `item-${index + 1}`,
    offer_id: asString(raw.offer_id),
    title: asString(raw.title) ?? asString(raw.name) ?? `Рекомендация ${index + 1}`,
    subtitle: asString(raw.subtitle) ?? asString(raw.description) ?? asString(raw.vendor),
    category: asString(raw.category),
    score: asNumber(raw.score) ?? 0,
    reason: asString(raw.reason) ?? asString(raw.explanation),
    price: asNumber(raw.price),
    currency: asString(raw.currency) ?? asString(raw.currency_code),
    hasHistory: raw.has_history === true,
    tags,
    image_url: asString(raw.image_url) ?? asString(raw.image),
    url: asString(raw.url) ?? asString(raw.link),
  };
}

function normalizeResponse(payload: unknown, fallbackUserId: string): RecommendationResponse {
  if (Array.isArray(payload)) {
    return {
      user_id: fallbackUserId,
      items: payload.map(normalizeItem),
      total: payload.length,
    };
  }

  if (!isRecord(payload)) {
    throw new RecsysError("Ответ API не является JSON-объектом");
  }

  const rawItems = Array.isArray(payload.items)
    ? payload.items
    : Array.isArray(payload.recommendations)
      ? payload.recommendations
      : null;

  if (!rawItems) {
    throw new RecsysError("В ответе API нет массива items");
  }

  return {
    user_id: asString(payload.user_id) ?? fallbackUserId,
    generated_at: asString(payload.generated_at),
    model_version: asString(payload.model_version),
    request_id: asString(payload.request_id),
    items: rawItems.map(normalizeItem),
    total: asNumber(payload.total) ?? rawItems.length,
    next_cursor: asString(payload.next_cursor) ?? null,
  };
}

function extractError(payload: unknown, status: number): string {
  if (isRecord(payload)) {
    const detail = asString(payload.detail) ?? asString(payload.message) ?? asString(payload.error);
    if (detail) return detail;
  }
  if (typeof payload === "string" && payload.trim()) {
    return payload.trim().slice(0, 300);
  }
  return `API ответил статусом ${status}`;
}

async function parseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  if (!trimmed) {
    throw new RecsysError("Не задан адрес API");
  }
  return trimmed.replace(/\/+$/, "");
}

async function request<T>(
  config: RecsysConfig,
  path: string,
  init: RequestInit = {},
  externalSignal?: AbortSignal,
): Promise<T> {
  const url = `${normalizeBaseUrl(config.baseUrl)}${path}`;
  const timeoutMs = config.timeoutMs ?? 8000;
  const controller = new AbortController();

  const timer = window.setTimeout(() => {
    controller.abort(new DOMException("Превышено время ожидания ответа API", "TimeoutError"));
  }, timeoutMs);

  const onAbort = () => controller.abort(externalSignal?.reason);
  if (externalSignal) {
    if (externalSignal.aborted) onAbort();
    else externalSignal.addEventListener("abort", onAbort);
  }

  const headers: Record<string, string> = {
    Accept: "application/json",
    ...(init.body ? { "Content-Type": "application/json" } : {}),
    ...(config.token?.trim() ? { Authorization: `Bearer ${config.token.trim()}` } : {}),
  };

  try {
    const response = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
    });

    const payload = await parseBody(response);

    if (!response.ok) {
      throw new RecsysError(extractError(payload, response.status), response.status, payload);
    }

    return payload as T;
  } catch (error) {
    if (error instanceof RecsysError) throw error;

    if (controller.signal.aborted) {
      const reason = controller.signal.reason;
      if (reason instanceof DOMException && reason.name === "TimeoutError") {
        throw new RecsysError(`API не ответил за ${timeoutMs} мс`);
      }
      throw new RecsysError("Запрос отменён");
    }

    throw new RecsysError(
      error instanceof Error ? error.message : "Не удалось выполнить запрос к API",
    );
  } finally {
    window.clearTimeout(timer);
    externalSignal?.removeEventListener("abort", onAbort);
  }
}

export interface StandSession {
  access_token: string;
  expires_at?: string;
}

/** Вход в синтетический аккаунт стенда: нужен, чтобы смена пользователя меняла выдачу. */
export async function loginStand(
  config: RecsysConfig,
  login: string,
  password: string,
): Promise<StandSession> {
  return request<StandSession>(config, "/auth/login", {
    method: "POST",
    body: JSON.stringify({ login, password }),
  });
}

export interface RecommendationParams {
  userId: string;
  limit?: number;
  scenario?: string;
  cursor?: string | null;
  signal?: AbortSignal;
}

export async function fetchRecommendations(
  config: RecsysConfig,
  params: RecommendationParams,
): Promise<RecommendationResponse> {
  // Текущий API берёт профиль из Bearer-сессии, а не из query user_id.
  const query = new URLSearchParams({ limit: String(params.limit ?? 12) });
  const payload = await request<unknown>(
    config,
    `/recommendations/me?${query.toString()}`,
    { method: "GET" },
    params.signal,
  );

  return normalizeResponse(payload, params.userId);
}

export async function sendFeedback(
  config: RecsysConfig,
  feedback: FeedbackPayload,
): Promise<FeedbackResponse> {
  if (feedback.action !== "click" && feedback.action !== "purchase") {
    return { accepted: true, mode: "local" };
  }

  let result: FeedbackResponse = { accepted: true };
  if (feedback.run_id && feedback.offer_id) {
    const event = await request<unknown>(config, "/events", {
      method: "POST",
      body: JSON.stringify({
        run_id: feedback.run_id,
        offer_id: feedback.offer_id,
        event_type: feedback.action,
      }),
    });
    result = isRecord(event) ? { ...event, accepted: true } : { accepted: true };
  }

  if (feedback.action === "purchase") {
    const purchase = await request<unknown>(config, "/purchases", {
      method: "POST",
      body: JSON.stringify({ item_id: feedback.item_id }),
    });
    result = isRecord(purchase) ? { ...purchase, ...result } : result;
  }
  return result;
}

export async function checkHealth(config: RecsysConfig): Promise<HealthResponse> {
  const payload = await request<unknown>(config, "/health", { method: "GET" });
  if (isRecord(payload)) {
    return {
      ...payload,
      status: asString(payload.status) ?? "ok",
      model_version: asString(payload.schema_version),
    };
  }
  return { status: "ok" };
}

export async function fetchProductDetail(
  config: RecsysConfig,
  itemID: string,
): Promise<ProductDetail> {
  const payload = await request<unknown>(
    config,
    `/products/${encodeURIComponent(itemID)}`,
    { method: "GET" },
  );
  const record = isRecord(payload) ? payload : {};
  const params = Array.isArray(record.parameters)
    ? record.parameters.map((entry) => {
        const item = isRecord(entry) ? entry : {};
        return { name: asString(item.name) ?? "", value: asString(item.value) ?? textValue(item.value) };
      })
    : [];
  return {
    item_id: asString(record.item_id) ?? itemID,
    offer_id: asString(record.offer_id) ?? "",
    sku: asString(record.sku) ?? "",
    title: asString(record.title) ?? itemID,
    category: asString(record.category) ?? "",
    vendor: asString(record.vendor) ?? "",
    price: asNumber(record.price),
    old_price: asNumber(record.old_price),
    currency_code: asString(record.currency_code) ?? "",
    available: record.available === true,
    url: asString(record.url) ?? "",
    description: asString(record.description) ?? "",
    picture_url: asString(record.picture_url) ?? "",
    vendor_url: asString(record.vendor_url) ?? "",
    delivery: record.delivery === true,
    pickup: record.pickup === true,
    sales_notes: asString(record.sales_notes) ?? "",
    parameters: params,
  };
}

export async function fetchScoreHistory(
  config: RecsysConfig,
  itemID: string,
): Promise<ScoreHistory> {
  const payload = await request<unknown>(
    config,
    `/products/${encodeURIComponent(itemID)}/score-history`,
    { method: "GET" },
  );
  const record = isRecord(payload) ? payload : {};
  const rawPoints = Array.isArray(record.points) ? record.points : [];
  return {
    item_id: asString(record.item_id) ?? itemID,
    average: asNumber(record.average) ?? 0,
    points: rawPoints.map((entry) => {
      const point = isRecord(entry) ? entry : {};
      return {
        at: asString(point.at) ?? "",
        score: asNumber(point.score) ?? 0,
        runs: asNumber(point.runs) ?? 0,
      };
    }),
  };
}

export interface ProductPage {
  items: RecommendationItem[];
  total: number;
  limit: number;
  offset: number;
}

export async function fetchProductsPage(
  config: RecsysConfig,
  params: { limit: number; offset: number; q?: string; withHistory?: boolean },
): Promise<ProductPage> {
  const query = new URLSearchParams({
    limit: String(params.limit),
    offset: String(params.offset),
  });
  if (params.q?.trim()) query.set("q", params.q.trim());
  if (params.withHistory) query.set("with_history", "1");
  const payload = await request<unknown>(config, `/products?${query.toString()}`, { method: "GET" });
  const record = isRecord(payload) ? payload : {};
  const rawItems = Array.isArray(record.items) ? record.items : [];
  return {
    items: rawItems.map((item, index) => normalizeItem(item, index)),
    total: asNumber(record.total) ?? rawItems.length,
    limit: asNumber(record.limit) ?? params.limit,
    offset: asNumber(record.offset) ?? params.offset,
  };
}

export interface DbStats {
  status: string;
  model_version?: string;
  tables: Record<string, number>;
}

export interface RunInfo {
  run_id: number;
  model_version: string;
  requested_at: string;
  items: RecommendationItem[];
  clicks: number;
  purchases: number;
}

export interface NewRunResponse extends RunInfo {
  client_id?: string;
}

export interface NewRunItem {
  item_id: string;
  offer_id?: string;
  explanation?: string;
  score?: number;
}

function normalizeRun(raw: unknown): RunInfo {
  const record = isRecord(raw) ? raw : {};
  const rawItems = Array.isArray(record.items) ? record.items : [];
  return {
    run_id: asNumber(record.run_id) ?? 0,
    model_version: asString(record.model_version) ?? "—",
    requested_at: asString(record.requested_at) ?? "",
    clicks: asNumber(record.clicks) ?? 0,
    purchases: asNumber(record.purchases) ?? 0,
    items: rawItems.map((item, index) => {
      const value = isRecord(item) ? item : {};
      return {
        item_id: asString(value.item_id) ?? `item-${index + 1}`,
        offer_id: asString(value.offer_id),
        title: asString(value.title) ?? asString(value.item_id) ?? `item-${index + 1}`,
        score: asNumber(value.score) ?? 0,
        reason: asString(value.explanation),
      };
    }),
  };
}

export async function fetchDbStats(config: RecsysConfig): Promise<DbStats> {
  const payload = await request<unknown>(config, "/stats", { method: "GET" });
  const record = isRecord(payload) ? payload : {};
  const rawTables = isRecord(record.tables) ? record.tables : {};
  const tables: Record<string, number> = {};
  for (const [name, value] of Object.entries(rawTables)) {
    const count = asNumber(value);
    if (count !== undefined) tables[name] = count;
  }
  return {
    status: asString(record.status) ?? "ok",
    model_version: asString(record.model_version),
    tables,
  };
}

export async function fetchRuns(
  config: RecsysConfig,
  _userId: string,
  limit = 10,
): Promise<{ user_id: string; client_id: string; runs: RunInfo[] }> {
  const payload = await request<unknown>(config, `/runs?limit=${limit}`, { method: "GET" });
  const record = isRecord(payload) ? payload : {};
  const rawRuns = Array.isArray(record.runs) ? record.runs : [];
  return {
    user_id: asString(record.user_id) ?? "—",
    client_id: asString(record.client_id) ?? "—",
    runs: rawRuns.map(normalizeRun),
  };
}

export async function createRun(
  config: RecsysConfig,
  items: NewRunItem[],
  modelVersion?: string,
): Promise<NewRunResponse> {
  const payload = await request<unknown>(config, "/runs", {
    method: "POST",
    body: JSON.stringify({
      model_version: modelVersion,
      items: items.slice(0, 3).map((item) => ({
        item_id: item.item_id,
        offer_id: item.offer_id ?? "",
        explanation: item.explanation ?? "Создано на стенде",
        score: item.score ?? 0,
      })),
    }),
  });
  const record = isRecord(payload) ? payload : {};
  return {
    ...normalizeRun(payload),
    client_id: asString(record.client_id),
  };
}
