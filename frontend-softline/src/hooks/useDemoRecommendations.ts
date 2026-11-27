import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, getRecommendations } from "../api/recsysClient";
import type { Recommendations } from "../api/recsysTypes";
import { useSession } from "../state/session";

export type DemoStatus = "idle" | "loading" | "success" | "empty" | "error";

export interface DemoRecommendationsState {
  status: DemoStatus;
  data: Recommendations | null;
  error: ApiError | null;
  requestId: string | null;
  elapsedMs: number | null;
  refresh: () => void;
}

const BASE_URL = import.meta.env.VITE_RECSYS_API_BASE ?? "/api/v1";

export function useDemoRecommendations(): DemoRecommendationsState {
  const session = useSession();
  const [status, setStatus] = useState<DemoStatus>("idle");
  const [data, setData] = useState<Recommendations | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [requestId, setRequestId] = useState<string | null>(null);
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);
  const [nonce, setNonce] = useState(0);
  // Последний рестарт API делает старый Bearer-токен недействительным:
  // перелогиниваемся один раз, чтобы экран восстановился сам.
  const retriedProfiles = useRef(new Set<string>());

  const refresh = useCallback(() => setNonce((value) => value + 1), []);

  useEffect(() => {
    const generation = session.generation;
    const profile = session.profile;

    // Смена профиля/выход: сразу убираем прежние данные.
    setData(null);
    setRequestId(null);
    setError(null);
    setElapsedMs(null);

    if (!profile) {
      setStatus("idle");
      return;
    }

    const controller = new AbortController();
    const started = performance.now();
    setStatus("loading");

    void getRecommendations(
      {
        baseUrl: BASE_URL,
        transport: session.transport,
        token: profile.token,
      },
      { scenario: session.scenario, profileId: profile.id, signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted || generation !== session.generation) return;
        setData(result);
        setRequestId(result.request_id);
        setElapsedMs(Math.round(performance.now() - started));
        setStatus(result.items.length ? "success" : "empty");
        setError(null);
      })
      .catch((cause: unknown) => {
        if (controller.signal.aborted || generation !== session.generation) return;
        const apiError =
          cause instanceof ApiError
            ? cause
            : new ApiError(
                cause instanceof Error ? cause.message : "Не удалось получить рекомендации",
                "client-error",
                "client_error",
              );
        if (
          apiError.status === 401 &&
          session.transport === "http" &&
          !retriedProfiles.current.has(profile.id)
        ) {
          retriedProfiles.current.add(profile.id);
          session.logout();
          return;
        }
        setError(apiError);
        setRequestId(apiError.requestId);
        setElapsedMs(Math.round(performance.now() - started));
        setStatus("error");
        setData(null);
      });

    return () => controller.abort();
  }, [
    session.profile?.id,
    session.profile?.token,
    session.transport,
    session.scenario,
    session.generation,
    session.logout,
    nonce,
  ]);

  return { status, data, error, requestId, elapsedMs, refresh };
}
