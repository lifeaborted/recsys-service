import { useCallback, useEffect, useState } from "react";
import { RecsysError, fetchRecommendations } from "../api/client";
import { mockRecommendations } from "../api/mock";
import type { RecommendationResponse, Scenario } from "../api/types";
import { useSettings } from "../state/settings";

export interface RecommendationsMeta {
  mode: "mock" | "live";
  durationMs: number;
  at: string;
  requestUrl: string;
}

export interface RecommendationsState {
  status: "idle" | "loading" | "success" | "error";
  data: RecommendationResponse | null;
  error: string | null;
  meta: RecommendationsMeta | null;
}

const INITIAL_STATE: RecommendationsState = {
  status: "idle",
  data: null,
  error: null,
  meta: null,
};

interface UseRecommendationsParams {
  userId: string;
  scenario: Scenario;
  limit: number;
  enabled?: boolean;
}

export function useRecommendations({
  userId,
  scenario,
  limit,
  enabled = true,
}: UseRecommendationsParams) {
  const { settings } = useSettings();
  const [state, setState] = useState<RecommendationsState>(INITIAL_STATE);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(() => setNonce((value) => value + 1), []);

  const { mockMode, baseUrl, token, timeoutMs } = settings;
  const normalizedUser = userId.trim();

  useEffect(() => {
    if (!enabled || !normalizedUser) {
      setState(INITIAL_STATE);
      return;
    }

    const controller = new AbortController();
    const startedAt = performance.now();
    const requestUrl = mockMode
      ? "mock://recommendations"
      : `${baseUrl.replace(/\/+$/, "")}/recommendations`;

    setState((previous) => ({
      status: "loading",
      data: previous.data,
      error: null,
      meta: previous.meta,
    }));

    const run = async () => {
      try {
        const data = mockMode
          ? await mockRecommendations({ userId: normalizedUser, limit, scenario })
          : await fetchRecommendations(
              { baseUrl, token, timeoutMs },
              { userId: normalizedUser, limit, scenario, signal: controller.signal },
            );

        if (controller.signal.aborted) return;

        setState({
          status: "success",
          data,
          error: null,
          meta: {
            mode: mockMode ? "mock" : "live",
            durationMs: Math.round(performance.now() - startedAt),
            at: new Date().toISOString(),
            requestUrl,
          },
        });
      } catch (error) {
        if (controller.signal.aborted) return;

        const message =
          error instanceof RecsysError || error instanceof Error
            ? error.message
            : "Не удалось загрузить рекомендации";

        setState({
          status: "error",
          data: null,
          error: message,
          meta: {
            mode: mockMode ? "mock" : "live",
            durationMs: Math.round(performance.now() - startedAt),
            at: new Date().toISOString(),
            requestUrl,
          },
        });
      }
    };

    void run();

    return () => controller.abort();
  }, [normalizedUser, scenario, limit, enabled, nonce, mockMode, baseUrl, token, timeoutMs]);

  return { ...state, refresh };
}
