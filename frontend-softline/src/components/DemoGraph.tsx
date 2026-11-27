import type { GraphEdge, GraphNode, Recommendation } from "../api/recsysTypes";

interface DemoGraphProps {
  item: Recommendation;
  edges: GraphEdge[];
  nodes: GraphNode[];
  showHead?: boolean;
}

const WIDTH = 840;
const BOX_WIDTH = 290;
const BOX_HEIGHT = 62;
const LEFT_X = 26;
const RIGHT_X = WIDTH - BOX_WIDTH - 26;
const ROW_HEIGHT = 96;

const TYPE_LABELS: Record<string, string> = {
  transition: "переход",
  basket: "совместная покупка",
  jaccard: "похожие товары",
  popularity: "популярное",
  catalog: "каталог",
  renewal: "продление",
  upgrade: "апгрейд",
  complement: "дополнение",
  behavioral: "по истории",
};

function typeLabel(type: string): string {
  return TYPE_LABELS[type] ?? type;
}

function unique(values: string[]): string[] {
  return [...new Set(values)];
}

function labelLines(value: string, limit = 26): [string, string?] {
  if (value.length <= limit) return [value];
  const words = value.split(" ");
  let first = "";
  let rest: string[] = [];
  for (let i = 0; i < words.length; i += 1) {
    const candidate = first ? `${first} ${words[i]}` : words[i];
    if (candidate.length > limit && first) {
      first = first.length > limit ? `${first.slice(0, limit - 1)}…` : first;
      rest = words.slice(i);
      break;
    }
    first = candidate;
  }
  if (!rest.length) return [value.length > limit ? `${value.slice(0, limit - 1)}…` : value];
  const second = rest.join(" ");
  return [first, second.length > limit ? `${second.slice(0, limit - 1)}…` : second];
}

/**
 * Граф только из рёбер, которые API вернул для этой карточки.
 * Узлы, их ID и подписи приходят из ответа — ничего не придумываем.
 */
export function DemoGraph({ item, edges, nodes, showHead = true }: DemoGraphProps) {
  if (edges.length === 0) {
    return (
      <div className="demo-graph demo-graph_empty">
        <p>Для этой карточки в ответе API нет рёбер графа.</p>
      </div>
    );
  }

  const labels = new Map(nodes.map((node) => [node.id, node.label]));
  const left = unique(edges.map((edge) => edge.from));
  const right = unique(edges.map((edge) => edge.to));
  const rows = Math.max(left.length, right.length, 1);
  const height = rows * ROW_HEIGHT + 18;
  const positions = new Map<string, { x: number; y: number }>();

  left.forEach((id, index) => {
    positions.set(id, { x: LEFT_X, y: 18 + index * ROW_HEIGHT });
  });
  right.forEach((id, index) => {
    if (!positions.has(id)) {
      positions.set(id, { x: RIGHT_X, y: 18 + index * ROW_HEIGHT });
    }
  });

  const markerId = `demo-arrow-${item.item_id.replace(/[^a-zA-Z0-9_-]/g, "-")}`;

  return (
    <div className="demo-graph" role="group" aria-label={`Граф связи: ${item.title}`}>
      {showHead ? (
        <div className="demo-graph__head">
          <strong>Граф связи</strong>
          <span>Связей: {edges.length} · только рёбра из ответа API</span>
        </div>
      ) : null}
      <svg
        className="demo-graph__canvas"
        viewBox={`0 0 ${WIDTH} ${height}`}
        role="img"
        aria-label={`Связи карточки ${item.title}`}
      >
        <defs>
          <marker id={markerId} markerWidth="10" markerHeight="10" refX="9" refY="3" orient="auto">
            <path d="M0,0 L0,6 L9,3 z" fill="#a20c33" />
          </marker>
        </defs>
        {edges.map((edge) => {
          const from = positions.get(edge.from);
          const to = positions.get(edge.to);
          if (!from || !to) return null;
          const x1 = from.x + BOX_WIDTH;
          const y1 = from.y + BOX_HEIGHT / 2;
          const x2 = to.x;
          const y2 = to.y + BOX_HEIGHT / 2;
          return (
            <g key={edge.id}>
              <line
                x1={x1}
                y1={y1}
                x2={x2}
                y2={y2}
                stroke="#a20c33"
                strokeWidth="1.8"
                markerEnd={`url(#${markerId})`}
              />
              <text
                x={(x1 + x2) / 2}
                y={(y1 + y2) / 2 - 9}
                textAnchor="middle"
                className="demo-graph__edge-label"
                stroke="#ffffff"
                strokeWidth={5}
                paintOrder="stroke"
              >
                {typeLabel(edge.type)}
              </text>
            </g>
          );
        })}
        {[...positions.entries()].map(([id, position]) => {
          const isTarget = id === `item:${item.item_id}`;
          const lines = labelLines(labels.get(id) ?? id);
          return (
            <g key={id}>
              <rect
                x={position.x}
                y={position.y}
                width={BOX_WIDTH}
                height={BOX_HEIGHT}
                rx="12"
                className={isTarget ? "demo-graph__node demo-graph__node_target" : "demo-graph__node"}
              />
              <text
                x={position.x + BOX_WIDTH / 2}
                y={position.y + BOX_HEIGHT / 2 - (lines[1] ? 6 : 0)}
                textAnchor="middle"
                className="demo-graph__node-label"
              >
                <tspan x={position.x + BOX_WIDTH / 2}>{lines[0]}</tspan>
                {lines[1] ? (
                  <tspan x={position.x + BOX_WIDTH / 2} dy="16">{lines[1]}</tspan>
                ) : null}
              </text>
            </g>
          );
        })}
      </svg>
      <ul className="demo-graph__edges">
        {edges.map((edge) => (
          <li key={edge.id}>
            <div className="demo-graph__edge-route">
              <span className="demo-graph__edge-type">{typeLabel(edge.type)}</span>
              <strong>{labels.get(edge.from) ?? edge.from}</strong>
              <span aria-hidden="true">→</span>
              <strong>{labels.get(edge.to) ?? edge.to}</strong>
            </div>
            <p className="demo-graph__edge-reason">{edge.reason}</p>
            <div className="demo-graph__edge-meta">
              <code>{edge.id}</code>
              <span>{edge.from} → {edge.to}</span>
              <span>{edge.source}</span>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
