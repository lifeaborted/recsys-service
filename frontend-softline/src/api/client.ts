import type {
  FeedbackPayload,
  FeedbackResponse,
  HealthResponse,
  RecommendationItem,
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
    title: asString(raw.title) ?? asString(raw.name) ?? `Рекомендация ${index + 1}`,
    subtitle: asString(raw.subtitle) ?? asString(raw.description),
    category: asString(raw.category),
    score: asNumber(raw.score) ?? 0,
    reason: asString(raw.reason) ?? asString(raw.explanation),
    price: asNumber(raw.price),
    currency: asString(raw.currency),
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
  const query = new URLSearchParams({
    user_id: params.userId,
    limit: String(params.limit ?? 12),
  });
  if (params.scenario) query.set("scenario", params.scenario);
  if (params.cursor) query.set("cursor", params.cursor);

  const payload = await request<unknown>(
    config,
    `/recommendations?${query.toString()}`,
    { method: "GET" },
    params.signal,
  );

  return normalizeResponse(payload, params.userId);
}

export async function sendFeedback(
  config: RecsysConfig,
  feedback: FeedbackPayload,
): Promise<FeedbackResponse> {
  const payload = await request<unknown>(config, "/feedback", {
    method: "POST",
    body: JSON.stringify(feedback),
  });

  if (isRecord(payload)) {
    return { ...payload, accepted: payload.accepted !== false };
  }

  return { accepted: true };
}

export async function checkHealth(config: RecsysConfig): Promise<HealthResponse> {
  const payload = await request<unknown>(config, "/health", { method: "GET" });
  if (isRecord(payload)) {
    return { ...payload, status: asString(payload.status) ?? "ok" };
  }
  return { status: "ok" };
}
