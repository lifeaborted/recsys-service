import { useCallback, useEffect, useState } from "react";
import {
  createRun,
  fetchDbStats,
  fetchRuns,
  RecsysError,
  sendFeedback,
  type DbStats,
  type NewRunItem,
  type RunInfo,
} from "../api/client";
import type { FeedbackPayload, RecommendationItem, Scenario } from "../api/types";
import { mockFeedback } from "../api/mock";
import { USERS } from "../data/account";
import { SCENARIO_OPTIONS } from "../data/scenarios";
import { useCloneRecommendations } from "../hooks/useCloneRecommendations";
import { formatDateTime, formatDuration, formatPrice } from "../lib/format";
import { useSettings } from "../state/settings";

export function RecommendationsPage() {
  const reco = useCloneRecommendations();
  const { settings, update } = useSettings();
  const [stats, setStats] = useState<DbStats | null>(null);
  const [runs, setRuns] = useState<RunInfo[]>([]);
  const [lastRunID, setLastRunID] = useState<number | null>(null);
  const [clientId, setClientId] = useState("—");
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const apiConfig = {
    baseUrl: settings.baseUrl,
    token: settings.token,
    timeoutMs: settings.timeoutMs,
  };

  const loadDb = useCallback(async () => {
    if (!settings.token) {
      setStats(null);
      setRuns([]);
      setClientId("—");
      return;
    }
    if (settings.mockMode) {
      setStats(null);
      setRuns([]);
      setClientId("—");
      return;
    }
    try {
      const [nextStats, nextRuns] = await Promise.all([
        fetchDbStats(apiConfig),
        fetchRuns(apiConfig, settings.userId),
      ]);
      setStats(nextStats);
      setRuns(nextRuns.runs);
      setClientId(nextRuns.client_id);
      setError(null);
    } catch (cause) {
      setError(
        cause instanceof RecsysError || cause instanceof Error
          ? cause.message
          : "Не удалось получить данные из БД",
      );
    }
    // apiConfig создаётся каждый рендер — зависимости заданы примитивами.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings.mockMode, settings.baseUrl, settings.token, settings.timeoutMs, settings.userId]);

  useEffect(() => {
    void loadDb();
  }, [loadDb]);

  const handleNewRun = async () => {
    setNotice(null);
    if (settings.mockMode) {
      setNotice("В mock-режиме новый run не создаётся — включите реальный API в Профиле.");
      return;
    }
    const items: NewRunItem[] = (reco.data?.items ?? []).slice(0, Math.min(3, settings.limit)).map((item) => ({
      item_id: item.item_id,
      offer_id: item.offer_id ?? "",
      explanation: item.reason ?? "Создано на стенде",
      score: item.score,
    }));
    try {
      const created = await createRun(apiConfig, items, stats?.model_version);
      setLastRunID(created.run_id);
      setNotice(`Создан run #${created.run_id} в БД, товаров: ${created.items.length}`);
      await loadDb();
      reco.refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Ошибка создания run");
    }
  };

  const handleFeedback = async (item: RecommendationItem, action: "click" | "purchase") => {
    const payload: FeedbackPayload = {
      user_id: settings.userId,
      item_id: item.item_id,
      offer_id: item.offer_id,
      run_id: lastRunID ?? runs[0]?.run_id,
      action,
      scenario: settings.scenario,
      request_id: reco.data?.request_id ?? null,
      ts: new Date().toISOString(),
      context: { source: "test-page", score: item.score },
    };
    try {
      if (settings.mockMode) await mockFeedback(payload);
      else await sendFeedback(apiConfig, payload);
      setNotice(`Событие «${action === "click" ? "click" : "purchase"}» записано в БД для ${item.item_id}`);
      await loadDb();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Ошибка записи события");
    }
  };

  const tableCounts = stats?.tables ?? {};

  return (
    <div className="react-page-inner">
      <h1>Тест рекомендаций</h1>
      <p className="react-page__lead">
        Полная проверка интеграции: БД → API → карточки → обратная связь. Все запросы и ответы
        видны в панели «⚙ Тест рекомендаций» и в консоли браузера.
      </p>

      <div className="stat-grid">
        <div className="stat-card">
          <div className="stat-card__value">{settings.mockMode ? "MOCK" : "LIVE"}</div>
          <div className="stat-card__label">режим</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value" title={stats?.model_version ?? "—"}>{stats?.model_version ?? "—"}</div>
          <div className="stat-card__label">модель</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value">{tableCounts.clients ?? "—"}</div>
          <div className="stat-card__label">клиентов в БД</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value">{tableCounts.catalog_offers ?? "—"}</div>
          <div className="stat-card__label">офферов</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value">{tableCounts.sales ?? "—"}</div>
          <div className="stat-card__label">продаж</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value">{tableCounts.recommendation_runs ?? "—"}</div>
          <div className="stat-card__label">run'ов</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value">{tableCounts.recommendation_events ?? "—"}</div>
          <div className="stat-card__label">событий</div>
        </div>
        <div className="stat-card">
          <div className="stat-card__value">{clientId}</div>
          <div className="stat-card__label">client_id</div>
        </div>
      </div>

      <div className="clone-card">
        <div className="form-grid">
          <div className="form-field">
            <label>Пользователь</label>
            <select value={settings.userId} onChange={(event) => update({ userId: event.target.value })}>
              {USERS.map((user) => (
                <option key={user.id} value={user.id}>
                  {user.name}
                </option>
              ))}
            </select>
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
            <select value={settings.limit} onChange={(event) => update({ limit: Number(event.target.value) })}>
              {[1, 2, 3, 4, 5, 6, 9, 12, 24, 36].map((value) => (
                <option key={value} value={value}>{value}</option>
              ))}
            </select>
          </div>
          <div className="form-field">
            <label>Режим</label>
            <input
              value={settings.mockMode ? "mock" : `${settings.baseUrl}`}
              readOnly
            />
          </div>
        </div>
        <div className="form-actions">
          <button
            className="button button_primary button_size-l"
            type="button"
            onClick={() => {
              setNotice("Выдача запрошена повторно");
              reco.refresh();
            }}
            disabled={reco.status === "loading"}
          >
            {reco.status === "loading" ? "Запрос…" : "Запросить выдачу"}
          </button>
          <button className="button button_default button_size-l" type="button" onClick={reco.refresh}>
            Применить лимит
          </button>
          <button
            className="button button_default button_size-l"
            type="button"
            onClick={() => void handleNewRun()}
          >
            Новый run
          </button>
        </div>
        <p className="form-note">
          {reco.meta
            ? `${reco.meta.mode} · ${formatDateTime(reco.meta.at)} · ${formatDuration(reco.meta.durationMs)} · ${reco.meta.requestUrl}`
            : "Запросов ещё не было"}
        </p>
        {!settings.mockMode && settings.scenario !== "demo" ? (
          <p className="form-note">Сценарий применён на клиенте: публичный API не принимает scenario.</p>
        ) : null}
        {notice ? <p className="form-note">{notice}</p> : null}
        {error ? <p className="form-note error">{error}</p> : null}
      </div>

      <div className="clone-card" style={{ marginTop: 20 }}>
        <h2 style={{ fontSize: 20, fontWeight: 600, margin: "0 0 16px" }}>Выдача</h2>
        <table className="clone-table">
          <thead>
            <tr>
              <th>#</th>
              <th>item_id</th>
              <th>Товар</th>
              <th>Категория</th>
              <th style={{ textAlign: "right" }}>Score</th>
              <th style={{ textAlign: "right" }}>Цена</th>
              <th>Событие</th>
            </tr>
          </thead>
          <tbody>
            {(reco.data?.items ?? []).map((item, index) => (
              <tr key={`${item.item_id}-${index}`}>
                <td>{index + 1}</td>
                <td>{item.item_id}</td>
                <td>
                  {item.title}
                  <div style={{ color: "#74767c", fontSize: 12 }}>{item.reason}</div>
                </td>
                <td>{item.category ?? "—"}</td>
                <td style={{ textAlign: "right" }}>{item.score.toFixed(3)}</td>
                <td style={{ textAlign: "right" }}>{formatPrice(item.price, item.currency)}</td>
                <td>
                  <div className="clone-actions">
                    <button
                      className="clone-link-btn"
                      type="button"
                      onClick={() => void handleFeedback(item, "click")}
                    >
                      click
                    </button>
                    <button
                      className="clone-link-btn"
                      type="button"
                      onClick={() => void handleFeedback(item, "purchase")}
                    >
                      purchase
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!reco.data?.items.length && reco.status !== "loading" ? (
          <div className="reco-empty">Нет данных для отображения</div>
        ) : null}
      </div>

      <div className="clone-card" style={{ marginTop: 20 }}>
        <h2 style={{ fontSize: 20, fontWeight: 600, margin: "0 0 16px" }}>Run'ы в БД</h2>
        <table className="clone-table">
          <thead>
            <tr>
              <th>run_id</th>
              <th>Время</th>
              <th>Модель</th>
              <th>Товары</th>
              <th style={{ textAlign: "right" }}>click</th>
              <th style={{ textAlign: "right" }}>purchase</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.run_id}>
                <td>{run.run_id}</td>
                <td>{formatDateTime(run.requested_at)}</td>
                <td>{run.model_version}</td>
                <td>
                  {run.items.map((item) => item.item_id).join(", ") || "—"}
                </td>
                <td style={{ textAlign: "right" }}>{run.clicks}</td>
                <td style={{ textAlign: "right" }}>{run.purchases}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!runs.length ? <div className="reco-empty">Run'ов пока нет — нажмите «Новый run»</div> : null}
      </div>
    </div>
  );
}
