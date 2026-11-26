import { useMemo, useState } from "react";
import { SCENARIO_OPTIONS } from "../data/scenarios";
import { formatDateTime, formatDuration } from "../lib/format";
import { useDebug } from "../state/debug";
import { useSettings } from "../state/settings";
import type { Scenario } from "../api/types";

type TabId = "request" | "response" | "error";

export function DebugPanel() {
  const debug = useDebug();
  const { settings, update } = useSettings();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<TabId>("request");
  const [copied, setCopied] = useState(false);

  const { snapshot } = debug;
  const text = useMemo(() => {
    const payload =
      tab === "request"
        ? snapshot?.request
        : tab === "response"
          ? snapshot?.response
          : snapshot?.error ?? null;
    if (payload === null || payload === undefined) return "";
    return typeof payload === "string" ? payload : JSON.stringify(payload, null, 2);
  }, [tab, snapshot]);

  const copy = async () => {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  };

  const status = snapshot?.meta
    ? `${snapshot.meta.mode === "mock" ? "MOCK" : "LIVE"} · ${settings.scenario} · ` +
      `${snapshot.items.length} товаров · ${formatDuration(snapshot.meta.durationMs)} · ` +
      `${formatDateTime(snapshot.meta.at)}`
    : snapshot?.error
      ? `Ошибка: ${snapshot.error}`
      : "Ожидание запроса…";

  return (
    <>
      <button type="button" className="clone-debug-toggle" onClick={() => setOpen((value) => !value)}>
        ⚙ Тест рекомендаций
      </button>
      <section
        className={`clone-debug${open ? " is-open" : ""}`}
        aria-label="Отладка рекомендаций"
      >
        <h3>Тест рекомендаций</h3>
        <p className="clone-debug__hint">
          Проверка БД и алгоритма. Запросы и ответы также видны в консоли браузера.
        </p>

        <div className="clone-debug__row">
          <div className="clone-debug__field">
            <label>Пользователь</label>
            <input
              value={settings.userId}
              onChange={(event) => update({ userId: event.target.value })}
            />
          </div>
          <div className="clone-debug__field">
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
          <div className="clone-debug__field" style={{ flex: "0 0 80px" }}>
            <label>Top-N</label>
            <input
              type="number"
              min={1}
              max={30}
              value={settings.limit}
              onChange={(event) => update({ limit: Math.max(1, Number(event.target.value) || 1) })}
            />
          </div>
        </div>

        <div className="clone-debug__row">
          <button type="button" className="clone-debug__btn" onClick={() => debug.rerun?.()}>
            Пересчитать
          </button>
          <button type="button" className="clone-debug__btn clone-debug__btn_secondary" onClick={debug.toggleScores}>
            Score: {debug.showScores ? "вкл" : "выкл"}
          </button>
          <button type="button" className="clone-debug__btn clone-debug__btn_secondary" onClick={() => void copy()}>
            {copied ? "Скопировано" : "Копировать JSON"}
          </button>
          <button type="button" className="clone-debug__btn clone-debug__btn_secondary" onClick={debug.clearHistory}>
            Очистить историю
          </button>
        </div>

        <div className={`clone-debug__status${snapshot?.error ? " error" : ""}`}>{status}</div>

        <table>
          <thead>
            <tr>
              <th>#</th>
              <th>Товар</th>
              <th>Причина</th>
              <th style={{ textAlign: "right" }}>Score</th>
            </tr>
          </thead>
          <tbody>
            {(snapshot?.items ?? []).map((item, index) => (
              <tr key={`${item.item_id}-${index}`}>
                <td>{index + 1}</td>
                <td>{item.title}</td>
                <td>{item.reason ?? "—"}</td>
                <td className="num">{item.score.toFixed(3)}</td>
              </tr>
            ))}
          </tbody>
        </table>

        <div className="clone-debug__section-title">JSON запроса / ответа</div>
        <div className="clone-debug__row" style={{ marginBottom: 8 }}>
          {(["request", "response", "error"] as TabId[]).map((id) => (
            <button
              key={id}
              type="button"
              className="clone-debug__btn clone-debug__btn_secondary"
              onClick={() => setTab(id)}
              disabled={id === "error" && !snapshot?.error}
            >
              {id === "request" ? "Запрос" : id === "response" ? "Ответ" : "Ошибка"}
            </button>
          ))}
        </div>
        <pre>{text || "Данных пока нет"}</pre>

        <div className="clone-debug__section-title">История вызовов (последние 20)</div>
        <div className="clone-debug__hint" style={{ whiteSpace: "pre-line" }}>
          {debug.history.length
            ? debug.history
                .map((entry) => `${entry.meta?.at ?? "—"} — ${entry.meta?.mode ?? "?"}, ${entry.items.length} шт.`)
                .join("\n")
            : "—"}
        </div>
      </section>
    </>
  );
}
