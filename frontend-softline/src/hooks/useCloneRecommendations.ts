import { useCallback, useEffect } from "react";
import { loginStand, sendFeedback } from "../api/client";
import { mockFeedback } from "../api/mock";
import type { FeedbackPayload, RecommendationItem, Scenario } from "../api/types";
import { demoCredentials } from "../data/account";
import { useDebug } from "../state/debug";
import { useSession } from "../state/session";
import { useSettings } from "../state/settings";
import { useRecommendations } from "./useRecommendations";

/** Общая точка входа: recommendation hook + отладка + обратная связь. */
export function useCloneRecommendations() {
  const { settings, update: updateSettings } = useSettings();
  const session = useSession();
  const { update: updateDebug, setRerun } = useDebug();

  // Первый вход берёт сессию демо-экрана, но дальше токен задаёт выбранный пользователь.
  useEffect(() => {
    const token = session.profile?.token;
    if (token && !settings.token) updateSettings({ token });
  }, [session.profile?.token, settings.token, updateSettings]);

  // Смена пользователя — это реальный вход в другой синтетический аккаунт.
  useEffect(() => {
    if (settings.mockMode) return;
    const credentials = demoCredentials(settings.userId);
    let cancelled = false;
    void loginStand(
      { baseUrl: settings.baseUrl, timeoutMs: settings.timeoutMs },
      credentials.login,
      credentials.password,
    ).then((result) => {
      if (!cancelled && result.access_token && result.access_token !== settings.token) {
        updateSettings({ token: result.access_token });
      }
    }).catch(() => {
      // Ошибку входа покажет сам запрос выдачи; здесь токен не трогаем.
    });
    return () => { cancelled = true; };
  }, [settings.userId, settings.mockMode, settings.baseUrl, settings.timeoutMs, updateSettings]);
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
