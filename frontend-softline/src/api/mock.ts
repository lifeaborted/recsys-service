import type {
  FeedbackPayload,
  FeedbackResponse,
  HealthResponse,
  RecommendationItem,
  RecommendationResponse,
  Scenario,
} from "./types";
import { CATALOG } from "../data/catalog";

const SCENARIO_REASONS: Record<Scenario, string[]> = {
  demo: ["Порядок выдачи lk.softline.ru (эталон)"],
  upsell: [
    "Похожие клиенты переходили на этот тариф в течение месяца",
    "Текущий лимит близок к исчерпанию — апгрейд снимет ограничение",
    "Модель оценила готовность к апгрейду выше среднего по сегменту",
  ],
  "cross-sell": [
    "Часто подключают вместе с сервисами из вашего портфеля",
    "Закрывает задачу, по которой вы обращались в поддержку",
    "Совместимо с текущей инфраструктурой без миграции",
  ],
  onboarding: [
    "Поможет быстрее запустить первый проект",
    "Снимает типовой блокер на старте — по данным похожих команд",
    "Входит в стартовый набор для вашего профиля",
  ],
  retention: [
    "Снижает риск оттока: закрывает частую причину обращений",
    "Повышает ценность подписки при том же бюджете",
    "Удерживает вовлечённость в сегменте с похожим поведением",
  ],
};

const SCENARIO_MODEL: Record<Scenario, string> = {
  demo: "mock-ranker/lk-demo-1.0",
  upsell: "mock-ranker/upsell-0.4",
  "cross-sell": "mock-ranker/cross-sell-0.4",
  onboarding: "mock-ranker/onboarding-0.3",
  retention: "mock-ranker/retention-0.3",
};

function hashString(input: string): number {
  let hash = 2166136261;
  for (let index = 0; index < input.length; index += 1) {
    hash ^= input.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

function mulberry32(seed: number): () => number {
  let state = seed;
  return () => {
    state |= 0;
    state = (state + 0x6d2b79f5) | 0;
    let t = Math.imul(state ^ (state >>> 15), 1 | state);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

export interface MockRecommendationParams {
  userId: string;
  limit: number;
  scenario: Scenario;
}

export async function mockRecommendations(
  params: MockRecommendationParams,
): Promise<RecommendationResponse> {
  await delay(180 + Math.random() * 220);

  const random = mulberry32(hashString(`${params.userId}:${params.scenario}`));
  const reasons = SCENARIO_REASONS[params.scenario];

  let items: RecommendationItem[];
  if (params.scenario === "demo") {
    items = [...CATALOG]
      .sort((left, right) => left.demoRank - right.demoRank)
      .map((item) => ({
        item_id: item.id,
        title: item.title,
        subtitle: item.vendor,
        category: item.category,
        score: Math.max(0.1, (100 - item.demoRank) / 100),
        reason: `порядок выдачи lk.softline.ru (позиция ${item.demoRank})`,
        price: item.price,
        currency: "RUB",
        tags: item.tags,
        image_url: item.image,
        url: item.url,
      }));
  } else {
    items = CATALOG.map((item) => {
      const score = Math.round((0.58 + random() * 0.41) * 1000) / 1000;
      const reason = reasons[Math.floor(random() * reasons.length)] ?? reasons[0];
      return {
        item_id: item.id,
        title: item.title,
        subtitle: item.vendor,
        category: item.category,
        score,
        reason,
        price: item.price,
        currency: "RUB",
        tags: item.tags,
        image_url: item.image,
        url: item.url,
      };
    }).sort((left, right) => right.score - left.score);
  }

  const limited = items.slice(0, Math.max(1, params.limit));
  return {
    user_id: params.userId,
    generated_at: new Date().toISOString(),
    model_version: SCENARIO_MODEL[params.scenario],
    request_id: `mock-${hashString(`${params.userId}:${params.scenario}:${Date.now()}`)}`,
    items: limited,
    total: limited.length,
    next_cursor: null,
  };
}

/**
 * Публичный OpenAPI не принимает scenario и user_id: выдача привязана к Bearer-сессии.
 * Чтобы селекторы на старых экранах работали, применяем сценарий и лимит на клиенте.
 * Пользователь при этом меняется реальным входом в другой синтетический аккаунт.
 */
export function applyLiveScenario(
  response: RecommendationResponse,
  params: { userId: string; scenario: Scenario; limit: number },
): RecommendationResponse {
  const limit = Math.max(1, params.limit);
  if (params.scenario === "demo") {
    const items = response.items.slice(0, limit);
    return { ...response, items, total: items.length };
  }

  const random = mulberry32(hashString(`${params.userId}:${params.scenario}`));
  const reasons = SCENARIO_REASONS[params.scenario];
  const items = response.items
    .map((item) => ({
      ...item,
      reason: reasons[Math.floor(random() * reasons.length)] ?? item.reason,
    }))
    .sort((left, right) => right.score - left.score)
    .slice(0, limit);

  return {
    ...response,
    model_version: SCENARIO_MODEL[params.scenario],
    items,
    total: items.length,
  };
}

export async function mockFeedback(payload: FeedbackPayload): Promise<FeedbackResponse> {
  await delay(120);
  return {
    accepted: true,
    event_id: `evt-${hashString(`${payload.item_id}:${payload.action}:${Date.now()}`)}`,
    stored: true,
  };
}

export async function mockHealth(): Promise<HealthResponse> {
  await delay(200);
  return {
    status: "ok",
    model_version: "mock-ranker/lk-demo-1.0",
    mode: "mock",
  };
}
