import { useCallback, useEffect } from "react";
import { sendFeedback } from "../api/client";
import { mockFeedback } from "../api/mock";
import type { FeedbackPayload, RecommendationItem, Scenario } from "../api/types";
import { useDebug } from "../state/debug";
import { useSession } from "../state/session";
import { useSettings } from "../state/settings";
import { useRecommendations } from "./useRecommendations";

/** Общая точка входа: recommendation hook + отладка + обратная связь. */
export function useCloneRecommendations() {
  const { settings, update: updateSettings } = useSettings();
  const session = useSession();
  const { update: updateDebug, setRerun } = useDebug();

  // Старые экраны должны использовать ту же Bearer-сессию, что и демо-экран.
  useEffect(() => {
    const token = session.profile?.token;
    if (token && token !== settings.token) updateSettings({ token });
  }, [session.profile?.token, settings.token, updateSettings]);
  const state = useRecommendations({
    userId: settings.userId,
    scenario: settings.scenario,
    limit: settings.limit,
  });

  useEffect(() => {
    updateDebug({
      request: {
        user_id: settings.userId,
        scenario: settings.scenario,
        limit: settings.limit,
        mode: settings.mockMode ? "mock" : "live",
        endpoint: settings.mockMode
          ? "mock://recommendations"
          : `${settings.baseUrl.replace(/\/+$/, "")}/recommendations/me`,
      },
      response: state.data,
      error: state.error,
      meta: state.meta,
      items: state.data?.items ?? [],
    });
  }, [
    updateDebug, state.status, state.data, state.error, state.meta,
    settings.userId, settings.scenario, settings.limit, settings.mockMode, settings.baseUrl,
  ]);

  useEffect(() => {
    setRerun(state.refresh);
    return () => setRerun(null);
  }, [setRerun, state.refresh]);

  const onFeedback = useCallback(
    (item: RecommendationItem) => {
      const payload: FeedbackPayload = {
        user_id: settings.userId,
        item_id: item.item_id,
        action: "click",
        scenario: settings.scenario as Scenario,
        request_id: state.data?.request_id ?? null,
        ts: new Date().toISOString(),
        context: { source: "lk-clone", score: item.score, placement: "main_page" },
      };
      if (settings.mockMode) {
        void mockFeedback(payload);
      } else {
        void sendFeedback(
          { baseUrl: settings.baseUrl, token: settings.token, timeoutMs: settings.timeoutMs },
          payload,
        ).catch((error: unknown) => {
          console.warn("[clone]反馈 не отправлен", error);
        });
      }
    },
    [settings, state.data?.request_id],
  );

  return { ...state, onFeedback };
}
