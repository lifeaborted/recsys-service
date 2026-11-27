import type { GraphEdge, GraphNode, Recommendation } from "../api/recsysTypes";

interface DemoGraphProps {
  item: Recommendation;
  edges: GraphEdge[];
  nodes: GraphNode[];
}

const WIDTH = 780;
const BOX_WIDTH = 250;
const BOX_HEIGHT = 50;
const LEFT_X = 28;
const RIGHT_X = WIDTH - BOX_WIDTH - 28;
const ROW_HEIGHT = 92;

function unique(values: string[]): string[] {
  return [...new Set(values)];
}

function shorten(value: string, limit: number): string {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value;
}

/**
 * Небольшой граф только из тех рёбер, которые API вернул для этой карточки.
 * Никаких товаров «из головы»: узлы и связи берутся из ответа.
 */
export function DemoGraph({ item, edges, nodes }: DemoGraphProps) {
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
  const height = Math.max(170, rows * ROW_HEIGHT + 40);
  const positions = new Map<string, { x: number; y: number }>();

  left.forEach((id, index) => {
    positions.set(id, { x: LEFT_X, y: 30 + index * ROW_HEIGHT });
  });
  right.forEach((id, index) => {
    if (!positions.has(id)) {
      positions.set(id, { x: RIGHT_X, y: 30 + index * ROW_HEIGHT });
    }
  });

  const markerId = `demo-arrow-${item.item_id.replace(/[^a-zA-Z0-9_-]/g, "-")}`;

  return (
    <div className="demo-graph" role="group" aria-label={`Граф связи: ${item.title}`}>
      <div className="demo-graph__head">
        <strong>Граф связи</strong>
        <span>Только рёбра из ответа API для этой карточки</span>
      </div>
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
                strokeWidth="1.5"
                markerEnd={`url(#${markerId})`}
              />
              <text
                x={(x1 + x2) / 2}
                y={(y1 + y2) / 2 - 10}
                textAnchor="middle"
                className="demo-graph__edge-label"
                stroke="#fafafb"
                strokeWidth={4}
                paintOrder="stroke"
              >
                {edge.type}
              </text>
            </g>
          );
        })}
        {[...positions.entries()].map(([id, position]) => {
          const isTarget = id === `item:${item.item_id}`;
          return (
            <g key={id}>
              <rect
                x={position.x}
                y={position.y}
                width={BOX_WIDTH}
                height={BOX_HEIGHT}
                rx="10"
                className={isTarget ? "demo-graph__node demo-graph__node_target" : "demo-graph__node"}
              />
              <text
                x={position.x + BOX_WIDTH / 2}
                y={position.y + BOX_HEIGHT / 2 + 4}
                textAnchor="middle"
                className="demo-graph__node-label"
              >
                {shorten(labels.get(id) ?? id, 30)}
              </text>
            </g>
          );
        })}
      </svg>
      <ul className="demo-graph__edges">
        {edges.map((edge) => (
          <li key={edge.id}>
            <span>{edge.from} → {edge.to}</span>
            <span> · {labels.get(edge.from) ?? edge.from} → {labels.get(edge.to) ?? edge.to}</span>
            <span> · {edge.type} · {edge.reason}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
