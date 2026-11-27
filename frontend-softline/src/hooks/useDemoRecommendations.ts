import { useCallback, useEffect, useState } from "react";
import { ApiError, getRecommendations } from "../api/recsysClient";
import type { Recommendations } from "../api/recsysTypes";
import { RECSYS_API, useSession } from "../state/session";

export function useDemoRecommendations(enabled = true) {
  const { profile, loading, logout } = useSession();
  const token = profile?.token ?? "";
  const [data, setData] = useState<Recommendations | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(true);
  const [nonce, setNonce] = useState(0);
  const refresh = useCallback(() => setNonce((value) => value + 1), []);

  useEffect(() => {
    setData(null);
    setError(null);
    if (loading || !enabled) return;
    const controller = new AbortController();
    setPending(true);
    void getRecommendations({ baseUrl: RECSYS_API, transport: "http", token },
      { scenario: "auto", profileId: profile?.profile_id ?? "guest", signal: controller.signal })
      .then((result) => { if (!controller.signal.aborted) setData(result); })
      .catch((cause: unknown) => {
        if (controller.signal.aborted) return;
        if (cause instanceof ApiError && cause.status === 401 && token) logout();
        else if (cause instanceof ApiError && cause.requestId !== "unknown") {
          setError(cause.message + " · Запрос " + cause.requestId);
        } else setError(cause instanceof Error ? cause.message : "Не удалось получить рекомендации");
      })
      .finally(() => { if (!controller.signal.aborted) setPending(false); });
    return () => controller.abort();
  }, [enabled, loading, logout, nonce, profile?.profile_id, token]);

  return { data, error, loading: loading || pending, refresh };
}
