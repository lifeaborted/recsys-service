import { SCENARIO_OPTIONS } from "../data/scenarios";
import { useCloneRecommendations } from "../hooks/useCloneRecommendations";
import { formatDateTime, formatDuration, formatPrice } from "../lib/format";
import { useSettings } from "../state/settings";
import type { Scenario } from "../api/types";

export function RecommendationsPage() {
  const reco = useCloneRecommendations();
  const { settings, update } = useSettings();

  return (
    <div className="react-page-inner">
      <h1>Тест рекомендаций</h1>
      <p className="react-page__lead">
        Проверка выдачи вашей БД: запрос, score, объяснения и обратная связь. Полный JSON обмена —
        в плавающей панели «⚙ Тест рекомендаций» и в консоли браузера.
      </p>

      <div className="clone-card">
        <div className="form-grid">
          <div className="form-field">
            <label>User ID</label>
            <input value={settings.userId} onChange={(event) => update({ userId: event.target.value })} />
          </div>
          <div className="form-field">
            <label>Сценарий</label>
            <select
              value={settings.scenario}
              onChange={(event) => update({ scenario: event.target.value as Scenario })}
            >
              {SCENARIO_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
          <div className="form-field">
            <label>Лимит</label>
            <input
              type="number"
              min={1}
              max={50}
              value={settings.limit}
              onChange={(event) => update({ limit: Math.max(1, Number(event.target.value) || 1) })}
            />
          </div>
          <div className="form-field">
            <label>Режим</label>
            <input value={settings.mockMode ? "mock" : `live · ${settings.baseUrl}`} readOnly />
          </div>
        </div>

        <div className="form-actions">
          <button
            type="button"
            className="button button_primary button_size-l"
            onClick={reco.refresh}
            disabled={reco.status === "loading"}
          >
            {reco.status === "loading" ? "Запрос…" : "Запросить выдачу"}
          </button>
        </div>

        <p className="form-note">
          {reco.meta
            ? `${reco.meta.mode === "mock" ? "MOCK" : "LIVE"} · ${formatDateTime(reco.meta.at)} · ${formatDuration(reco.meta.durationMs)} · ${reco.meta.requestUrl}`
            : "Запросов ещё не было"}
        </p>
        {reco.error ? <p className="form-note error">{reco.error}</p> : null}
      </div>

      <div className="clone-card" style={{ marginTop: 20 }}>
        <table className="clone-table">
          <thead>
            <tr>
              <th>#</th>
              <th>Товар</th>
              <th>Категория</th>
              <th>Причина</th>
              <th style={{ textAlign: "right" }}>Score</th>
              <th style={{ textAlign: "right" }}>Цена</th>
            </tr>
          </thead>
          <tbody>
            {(reco.data?.items ?? []).map((item, index) => (
              <tr key={`${item.item_id}-${index}`}>
                <td>{index + 1}</td>
                <td>{item.title}</td>
                <td>{item.category ?? "—"}</td>
                <td>{item.reason ?? "—"}</td>
                <td className="num">{item.score.toFixed(3)}</td>
                <td style={{ textAlign: "right" }}>{formatPrice(item.price, item.currency)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!reco.data?.items.length && reco.status !== "loading" ? (
          <div className="reco-empty">Нет данных для отображения</div>
        ) : null}
      </div>
    </div>
  );
}
