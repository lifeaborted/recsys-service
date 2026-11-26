export type Scenario = "demo" | "upsell" | "cross-sell" | "onboarding" | "retention";

export type FeedbackAction = "click" | "like" | "dislike" | "hide" | "purchase";

export interface RecommendationItem {
  item_id: string;
  title: string;
  subtitle?: string;
  category?: string;
  score: number;
  reason?: string;
  price?: number;
  currency?: string;
  tags?: string[];
  image_url?: string;
  url?: string;
}

export interface RecommendationResponse {
  user_id: string;
  generated_at?: string;
  model_version?: string;
  request_id?: string;
  items: RecommendationItem[];
  total?: number;
  next_cursor?: string | null;
}

export interface FeedbackPayload {
  user_id: string;
  item_id: string;
  action: FeedbackAction;
  scenario?: Scenario;
  request_id?: string | null;
  ts: string;
  context?: Record<string, unknown>;
}

export interface FeedbackResponse {
  accepted: boolean;
  [key: string]: unknown;
}

export interface HealthResponse {
  status?: string;
  model_version?: string;
  [key: string]: unknown;
}
