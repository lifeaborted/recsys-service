import { useEffect, useState } from "react";
import { getRecommendationDiagnostics, type DebugTarget, type RecommendationDiagnostics } from "../api/recsysClient";
import type { GraphEdge, GraphNode, Recommendation } from "../api/recsysTypes";
import { DemoGraphModal } from "../components/DemoGraphModal";
import { DemoMetricsPanel } from "../components/DemoMetricsPanel";
import { DemoRecommendationCard } from "../components/DemoRecommendationCard";
import { RECSYS_API, useSession } from "../state/session";
import "./preview.css";

const client = (token: string) => ({ baseUrl: RECSYS_API, transport: "http" as const, token });
const profiles: ReadonlyArray<{ id: DebugTarget; label: string }> = [
  { id: "guest", label: "Гость" },
  { id: "notver", label: "Неверифицированный" },
  { id: "verb2b", label: "Верифицированный B2B" },
  { id: "verb2c", label: "Верифицированный B2C" },
  { id: "admin", label: "Администратор" },
];

function AdminDiagnostics({ token }: { token: string }) {
  const [target, setTarget] = useState<DebugTarget>("guest");
  const [data, setData] = useState<RecommendationDiagnostics | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const [graph, setGraph] = useState<{ item: Recommendation; edges: GraphEdge[]; nodes: GraphNode[] } | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setData(null);
    setError(null);
    setGraph(null);
    void getRecommendationDiagnostics(client(token), target, controller.signal).then((result) => {
      if (!controller.signal.aborted) setData(result);
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "Ошибка диагностики");
    });
    return () => controller.abort();
  }, [revision, target, token]);

  return (
    <>
      <h1>Отладка рекомендаций</h1>
      <div className="preview-controls">
        <label>
          Профиль для проверки
          <select value={target} onChange={(event) => setTarget(event.target.value as DebugTarget)}>
            {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.label}</option>)}
          </select>
        </label>
        <button className="clone-link-btn clone-link-btn_primary" type="button" onClick={() => setRevision((n) => n + 1)}>Обновить</button>
      </div>
      {error ? <p role="alert">{error}</p> : !data ? <p role="status">Загрузка диагностики…</p> : (
        <>
          <p className="preview-meta">
            Снимок <b>{data.recommendations.snapshot_id}</b> · {data.duration_ms} мс · запрос {data.recommendations.request_id}
          </p>
          <div className="preview-grid">
            {data.recommendations.items.map((item) => {
              const edges = data.recommendations.graph.edges.filter((edge) => item.evidence_ids.includes(edge.id));
              return <DemoRecommendationCard key={item.item_id} item={item} edges={edges} busy={false}
                onShowGraph={() => setGraph({ item, edges, nodes: data.recommendations.graph.nodes })} />;
            })}
          </div>
          <details open>
            <summary>Счётчики базы</summary>
            <dl className="preview-stats">
              {Object.entries(data.tables ?? {}).map(([name, count]) => <div key={name}><dt>{name}</dt><dd>{count}</dd></div>)}
            </dl>
          </details>
          <details>
            <summary>Ответ ранжирования, оценки и граф</summary>
            <pre>{JSON.stringify(data.recommendations, null, 2)}</pre>
          </details>
          <details>
            <summary>Сохранённые прогоны и события</summary>
            {data.runs.length ? <pre>{JSON.stringify(data.runs, null, 2)}</pre> : <p>Для выбранного профиля сохранённых прогонов нет.</p>}
          </details>
        </>
      )}
      <details>
        <summary>Открытый источник: сравнение тарифов Microsoft 365</summary>
        <p>Business Standard включает настольные приложения. Business Basic предлагает веб- и мобильные версии. Для клиента, которому нужны настольные приложения, это проверяемое основание рассмотреть переход на Standard.</p>
        <p>Qwen предложила связь «дополнение». Проверка источника подтвердила разницу функций, поэтому предложение о дополнении осталось на проверке.</p>
        <a href="https://www.microsoft.com/en/microsoft-365/business/with-teams-plans-and-pricing" target="_blank" rel="noreferrer">Таблица сравнения Microsoft</a>
      </details>
      <DemoMetricsPanel />
      {graph ? <DemoGraphModal {...graph} onClose={() => setGraph(null)} /> : null}
    </>
  );
}

export function DiagnosticsPage() {
  const { profile } = useSession();
  return (
    <div className="react-page-inner portal-diagnostics">
      {profile?.role === "admin"
        ? <AdminDiagnostics token={profile.token} />
        : <p role="status">Для просмотра диагностики нужен аккаунт администратора.</p>}
    </div>
  );
}
