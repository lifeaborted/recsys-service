import { useEffect } from "react";
import type { GraphEdge, GraphNode, Recommendation } from "../api/recsysTypes";
import { DemoGraph } from "./DemoGraph";

interface DemoGraphModalProps {
  item: Recommendation;
  edges: GraphEdge[];
  nodes: GraphNode[];
  onClose: () => void;
}

/**
 * Отдельное окно для графа: на карточке схема была слишком мелкой.
 * Закрывается кнопкой, кликом по фону и Escape.
 */
export function DemoGraphModal({ item, edges, nodes, onClose }: DemoGraphModalProps) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const previousOverflow = document.body.style.overflow;
    document.addEventListener("keydown", onKeyDown);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
    };
  }, [onClose]);

  return (
    <div
      className="demo-modal"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        className="demo-modal__window"
        role="dialog"
        aria-modal="true"
        aria-labelledby="demo-graph-modal-title"
      >
        <div className="demo-modal__head">
          <div>
            <h2 id="demo-graph-modal-title">Граф связи</h2>
            <p className="demo-modal__subtitle">{item.title} · {item.item_id}</p>
          </div>
          <button className="demo-modal__close" type="button" aria-label="Закрыть граф" onClick={onClose}>
            ×
          </button>
        </div>
        <DemoGraph item={item} edges={edges} nodes={nodes} />
        <div className="demo-modal__foot">
          <span>Все узлы и рёбра взяты из ответа API для этой карточки.</span>
          <button className="clone-link-btn" type="button" onClick={onClose}>Закрыть</button>
        </div>
      </div>
    </div>
  );
}
