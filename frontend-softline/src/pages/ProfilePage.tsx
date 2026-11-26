import { useState } from "react";
import { checkHealth, RecsysError } from "../api/client";
import { mockHealth } from "../api/mock";
import type { Scenario } from "../api/types";
import { USERS } from "../data/account";
import { SCENARIO_OPTIONS } from "../data/scenarios";
import { useSettings } from "../state/settings";

export function ProfilePage() {
  const { settings, update, reset } = useSettings();
  const [health, setHealth] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const check = async () => {
    setBusy(true);
    setHealth(null);
    try {
      const result = settings.mockMode
        ? await mockHealth()
        : await checkHealth({ baseUrl: settings.baseUrl, token: settings.token, timeoutMs: settings.timeoutMs });
      setHealth(`OK · ${result.status ?? "ok"} · ${result.model_version ?? "—"}`);
    } catch (error) {
      setHealth(
        error instanceof RecsysError || error instanceof Error
          ? `Ошибка: ${error.message}`
          : "Ошибка проверки соединения",
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="react-page-inner">
      <h1>Профиль</h1>
      <p className="react-page__lead">
        Настройки подключенияrecommendation-сервиса. Изменения сохраняются автоматически.
      </p>

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
            <label>Лимит выдачи</label>
            <input
              type="number"
              min={1}
              max={50}
              value={settings.limit}
              onChange={(event) => update({ limit: Math.max(1, Number(event.target.value) || 1) })}
            />
          </div>
          <div className="form-field">
            <label>Таймаут, мс</label>
            <input
              type="number"
              min={500}
              step={500}
              value={settings.timeoutMs}
              onChange={(event) => update({ timeoutMs: Math.max(500, Number(event.target.value) || 500) })}
            />
          </div>
          <div className="form-field">
            <label>Базовый URL API</label>
            <input
              value={settings.baseUrl}
              onChange={(event) => update({ baseUrl: event.target.value })}
              placeholder="/api"
            />
          </div>
          <div className="form-field">
            <label>Bearer-токен (опционально)</label>
            <input
              type="password"
              value={settings.token}
              onChange={(event) => update({ token: event.target.value })}
            />
          </div>
        </div>

        <label style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <input
            type="checkbox"
            checked={!settings.mockMode}
            onChange={(event) => update({ mockMode: !event.target.checked })}
          />
          Использовать реальный API вместо mock
        </label>

        <div className="form-actions" style={{ marginTop: 20 }}>
          <button className="button button_primary button_size-l" type="button" onClick={() => void check()} disabled={busy}>
            {busy ? "Проверяем…" : "Проверить соединение"}
          </button>
          <button className="button button_default button_size-l" type="button" onClick={reset}>
            Сбросить настройки
          </button>
          <a className="button button_default button_size-l" href="#/recommendations">
            Тест рекомендаций
          </a>
        </div>

        {health ? <p className={`form-note${health.startsWith("Ошибка") ? " error" : ""}`}>{health}</p> : null}
        <p className="form-note">
          Vite проксирует <code>/api</code> на адрес из <code>VITE_PROXY_TARGET</code>. Для mock-режима
          сервер не нужен. Не используйте реальные секреты на публичном стенде.
        </p>
      </div>
    </div>
  );
}
