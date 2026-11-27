/** Типы по api/openapi.yaml (вариант 5 — демо). */
export type RecommendationKind = "renewal" | "upgrade" | "complement" | "behavioral" | "popular";

export interface Difference {
  field: string;
  value: unknown;
  unit: string;
  source: string;
}

export interface Variant {
  item_id: string;
  url?: string;
  differences: Difference[];
}

export type VerificationStatus = "confirmed" | "review";

export interface Recommendation {
  item_id: string;
  offer_id?: string;
  sku: string | null;
  recommendation_kind: RecommendationKind;
  family_id: string | null;
  title: string;
  url?: string;
  explanation: string;
  variants: Variant[];
  evidence_ids: string[];
  /**
   * Демо-пометка о том, что совместимость подтверждена или требует проверки.
   * Боевой API это поле пока не отдаёт, поэтому оно необязательное.
   */
  status?: VerificationStatus;
}

export interface GraphNode {
  id: string;
  type: string;
  label: string;
}

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  type: string;
  weight: number;
  reason: string;
  source: string;
  snapshot_id: string;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface Recommendations {
  request_id: string;
  snapshot_id: string;
  partial_reason: string | null;
  items: Recommendation[];
  graph: Graph;
}

export interface ApiErrorPayload {
  request_id: string;
  error: { code: string; message: string };
}

export interface SessionResponse {
  access_token: string;
  expires_at: string;
  profile_id?: string;
}

export interface MeResponse {
  role: "unverified" | "verified" | "admin";
  profile_id: string;
  client_id: string;
  client_group: "B2B" | "B2C";
  login: string;
}

export type DemoScenario = "full" | "partial" | "empty" | "error" | "slow" | "check" | "auto";
