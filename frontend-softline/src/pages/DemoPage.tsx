import { useEffect, useState } from "react";
import { listProducts, purchase, type CatalogProduct } from "../api/recsysClient";
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
  const [catalog, setCatalog] = useState<CatalogProduct[]>([]);
  const [selectedItem, setSelectedItem] = useState("");

  const cfg = {
    baseUrl: import.meta.env.VITE_RECSYS_API_BASE ?? "/api/v1",
    transport: session.transport,
    token: session.profile?.token ?? "",
  };

  useEffect(() => {
    setCatalog([]);
    setSelectedItem("");
    if (cfg.transport !== "http" || !cfg.token) return;
    const controller = new AbortController();
    listProducts(cfg, controller.signal).then(
      (data) => setCatalog(data.items.filter((item) => item.available)),
      (error: Error) => { if (!controller.signal.aborted) setNotice(error.message); },
    );
    return () => controller.abort();
  }, [cfg.baseUrl, cfg.transport, cfg.token]);

  const buy = async (itemId: string) => {
    setBusy(true);
    try {
      await purchase(cfg, itemId);
      setNotice(`Покупка записана для ${itemId}`);
      demo.refresh();
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : "Ошибка записи покупки");
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
              <option value="" disabled>Выберите профиль</option>
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
            onClick={demo.refresh}
            disabled={busy}
          >
            Пересчитать выдачу
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

        {session.error ? <p role="alert">{session.error}</p> : null}
        {catalog.length ? (
          <div className="demo-toolbar">
            <label>Каталог <select value={selectedItem} onChange={(event) => setSelectedItem(event.target.value)}>
              <option value="">Выберите товар</option>
              {catalog.map((item) => <option key={item.item_id} value={item.item_id}>{item.title}</option>)}
            </select></label>
            <button type="button" disabled={!selectedItem || busy} onClick={() => void buy(selectedItem)}>Купить из каталога</button>
          </div>
        ) : null}
        {notice ? <p className="demo-status">{notice}</p> : null}

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
                [item.item_id, ...item.variants.map((variant) => variant.item_id)]
                  .some((id) => edge.to === `item:${id}` || edge.from === `item:${id}`),
            );
            return (
              <DemoRecommendationCard key={item.item_id} item={item} edges={edges} onPurchase={buy} busy={busy} />
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
