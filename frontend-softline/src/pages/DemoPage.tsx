import { useState } from "react";
import { sendFeedback, newRun } from "../api/recsysClient";
import type { DemoScenario } from "../api/recsysTypes";
import { DemoRecommendationCard } from "../components/DemoRecommendationCard";
import { useDemoRecommendations } from "../hooks/useDemoRecommendations";
import { formatDuration } from "../lib/format";
import { DEMO_PROFILES, useSession } from "../state/session";
import type { Transport } from "../api/recsysClient";

const SCENARIOS: Array<{ value: DemoScenario; label: string }> = [
  { value: "auto", label: "Авто (по профилю)" },
  { value: "full", label: "Полная выдача" },
  { value: "partial", label: "Частичная" },
  { value: "empty", label: "Пустая" },
  { value: "error", label: "Ошибка" },
  { value: "slow", label: "Медленный ответ" },
];

export function DemoPage() {
  const session = useSession();
  const demo = useDemoRecommendations();
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const cfg = {
    baseUrl: import.meta.env.VITE_RECSYS_API_BASE ?? "/api/v1",
    transport: session.transport,
    token: session.profile?.token ?? "",
  };

  const onFeedback = async (itemId: string, action: "click" | "purchase") => {
    try {
      await sendFeedback(cfg, itemId, action);
      setNotice(`Событие «${action}» записано для ${itemId}`);
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : "Ошибка записи события");
    }
  };

  const onCreateRun = async () => {
    setBusy(true);
    try {
      await newRun(cfg);
      setNotice("Новый run создан");
      demo.refresh();
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : "Ошибка создания run");
    } finally {
      setBusy(false);
    }
  };

  const items = demo.data?.items.slice(0, 3) ?? [];
  const graph = demo.data?.graph;

  return (
    <div className="react-page-inner">
      <h1>Рекомендации</h1>
      <p className="react-page__lead">
        Демо-экран личного кабинета. До трёх карточек с объяснениями, сравнением вариантов и графом.
      </p>

      <div className="clone-card">
        <div className="demo-toolbar">
          <label>
            Профиль{" "}
            <select
              value={session.profile?.id ?? ""}
              onChange={(event) => session.loginAs(event.target.value)}
            >
              {DEMO_PROFILES.map((profile) => (
                <option key={profile.id} value={profile.id}>
                  {profile.group} · {profile.login}
                </option>
              ))}
            </select>
          </label>
          <label>
            Транспорт{" "}
            <select
              value={session.transport}
              onChange={(event) => session.setTransport(event.target.value as Transport)}
            >
              <option value="mock">mock (синтетика)</option>
              <option value="http">http (/api/v1)</option>
            </select>
          </label>
          <label>
            Сценарий{" "}
            <select
              value={session.scenario}
              onChange={(event) => session.setScenario(event.target.value as DemoScenario)}
            >
              {SCENARIOS.map((scenario) => (
                <option key={scenario.value} value={scenario.value}>
                  {scenario.label}
                </option>
              ))}
            </select>
          </label>
          <button className="button button_primary button_size-l" type="button" onClick={demo.refresh}>
            Обновить
          </button>
          <button
            className="button button_default button_size-l"
            type="button"
            onClick={() => void onCreateRun()}
            disabled={busy}
          >
            Новый run
          </button>
          <button className="button button_default button_size-l" type="button" onClick={session.logout}>
            Выйти
          </button>
        </div>

        <p className="demo-status">
          {session.profile ? `Сессия: ${session.profile.group} (${session.profile.id})` : "Сессия не активна"}
          {demo.requestId ? ` · request_id: ${demo.requestId}` : ""}
          {demo.elapsedMs !== null ? ` · ${formatDuration(demo.elapsedMs)}` : ""}
          {demo.data ? ` · snapshot: ${demo.data.snapshot_id}` : ""}
        </p>

        {notice ? <p className="demo-status">{notice}</p> : null}

        {demo.data ? (
          <p className="demo-status">
            Доли попаданий (синтетический набор {demo.data.snapshot_id}, каталог и профили от 26.11.2026):{" "}
            другой артикул знакомого семейства — {Math.round(items.filter((item) => item.family_id).length / Math.max(items.length, 1) * 100)}%;{" "}
            новые продукты — {Math.round(items.filter((item) => !item.family_id).length / Math.max(items.length, 1) * 100)}%;{" "}
            неизвестное семейство — 0%. Это демонстрационные доли по выбранной выдаче; агрегаты
            качества участника 1 в открытом стенде не используются.
          </p>
        ) : null}

        {demo.data?.partial_reason ? (
          <div className="demo-banner">
            Частичная выдача: {demo.data.partial_reason}. Показано {items.length} из возможных Recommendation.
          </div>
        ) : null}

        {demo.status === "error" && demo.error ? (
          <div className="demo-banner demo-banner_error">
            Ошибка {demo.error.code} (request_id: {demo.error.requestId}): {demo.error.message}
          </div>
        ) : null}

        {demo.status === "empty" ? <div className="demo-status">Нет рекомендаций для этого профиля.</div> : null}
        {demo.status === "loading" ? <div className="demo-status">Загружаем рекомендации…</div> : null}
        {demo.status === "idle" ? <div className="demo-status">Войдите в демо-профиль, чтобы увидеть выдачу.</div> : null}
      </div>

      {items.length > 0 ? (
        <div className="demo-cards" style={{ marginTop: 20 }}>
          {items.map((item) => {
            const edges = (graph?.edges ?? []).filter(
              (edge) =>
                item.evidence_ids.includes(edge.id) &&
                (edge.to === `item:${item.item_id}` || edge.from === `item:${item.item_id}`),
            );
            return (
              <DemoRecommendationCard key={item.item_id} item={item} edges={edges} onFeedback={onFeedback} />
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
