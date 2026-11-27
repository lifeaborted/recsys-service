export type Scenario = "demo" | "upsell" | "cross-sell" | "onboarding" | "retention";

export type FeedbackAction = "click" | "like" | "dislike" | "hide" | "purchase";

export interface RecommendationItem {
  item_id: string;
  offer_id?: string;
  title: string;
  subtitle?: string;
  category?: string;
  score: number;
  reason?: string;
  price?: number;
  currency?: string;
  hasHistory?: boolean;
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
  offer_id?: string;
  run_id?: number;
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

export interface ProductDetail {
  item_id: string;
  offer_id: string;
  sku: string;
  title: string;
  category: string;
  vendor: string;
  price?: number;
  old_price?: number;
  currency_code: string;
  available: boolean;
  url: string;
  description: string;
  picture_url: string;
  vendor_url: string;
  delivery: boolean;
  pickup: boolean;
  sales_notes: string;
  parameters: Array<{ name: string; value: string }>;
}

export interface ScorePoint {
  at: string;
  score: number;
  runs: number;
}

export interface ScoreHistory {
  item_id: string;
  average: number;
  points: ScorePoint[];
}

export interface HealthResponse {
  status?: string;
  model_version?: string;
  [key: string]: unknown;
}
