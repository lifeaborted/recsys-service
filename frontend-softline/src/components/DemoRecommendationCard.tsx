import { useState } from "react";
import type { GraphEdge, Recommendation, Variant } from "../api/recsysTypes";

interface DemoRecommendationCardProps {
  item: Recommendation;
  edges: GraphEdge[];
  onFeedback: (itemId: string, action: "click" | "purchase") => void;
}

const KIND_LABELS: Record<Recommendation["recommendation_kind"], string> = {
  renewal: "продление",
  upgrade: "апгрейд",
  complement: "дополнение",
  behavioral: "по истории",
  popular: "популярное",
};

function termOf(variant: Variant): string {
  const term = variant.differences.find((difference) => difference.field === "term")?.value;
  return term == null ? "срок не указан" : `${term} мес.`;
}

function expiryOf(variant: Variant): string | null {
  const difference = variant.differences.find((entry) => entry.field === "expires_at");
  if (!difference) return null;
  if (difference.value === null || difference.value === undefined || difference.value === "") {
    return "Срок окончания неизвестен";
  }
  return `до ${String(difference.value)}`;
}

export function DemoRecommendationCard({ item, edges, onFeedback }: DemoRecommendationCardProps) {
  const [showVariants, setShowVariants] = useState(false);
  const [showReason, setShowReason] = useState(false);

  return (
    <article className="demo-card">
      <span className="demo-card__kind">{KIND_LABELS[item.recommendation_kind]}</span>
      <h3 className="demo-card__title">{item.title}</h3>
      <p className="demo-card__explanation">{item.explanation}</p>
      {item.url ? (
        <a className="demo-card__link" href={item.url} target="_blank" rel="noreferrer">
          Открыть карточку →
        </a>
      ) : (
        <span className="demo-card__link">Ссылка недоступна</span>
      )}

      {item.variants.length > 0 ? (
        <div className="demo-card__section">
          <button className="clone-link-btn" type="button" onClick={() => setShowVariants((value) => !value)}>
            {showVariants ? "Скрыть варианты" : "Сравнить варианты"}
          </button>
          {showVariants ? (
            <div style={{ marginTop: 10 }}>
              {item.variants.map((variant) => {
                const expiry = expiryOf(variant);
                return (
                  <div className="demo-variant" key={variant.item_id}>
                    <div className="demo-variant__term">
                      {variant.item_id} · {termOf(variant)}
                    </div>
                    <div>
                      {variant.differences
                        .filter((difference) => difference.field !== "term")
                        .map((difference) => (
                          <span key={`${difference.field}-${difference.source}`}>
                            {difference.field}: {String(difference.value ?? "неизвестно")} ({difference.source}){" "}
                          </span>
                        ))}
                    </div>
                    {expiry ? (
                      <div className={expiry.startsWith("Срок") ? "demo-variant__unknown" : ""}>{expiry}</div>
                    ) : null}
                    {variant.url ? (
                      <a className="demo-card__link" href={variant.url} target="_blank" rel="noreferrer">
                        Вариант →
                      </a>
                    ) : null}
                  </div>
                );
              })}
            </div>
          ) : null}
        </div>
      ) : null}

      {edges.length > 0 ? (
        <div className="demo-card__section">
          <button className="clone-link-btn" type="button" onClick={() => setShowReason((value) => !value)}>
            {showReason ? "Скрыть причину" : "Почему рекомендуем"}
          </button>
          {showReason ? (
            <div style={{ marginTop: 10 }}>
              {edges.map((edge) => (
                <div key={edge.id}>
                  <p className="demo-card__reason">{edge.reason}</p>
                  <div className="demo-card__source">
                    {edge.type} · {edge.source} · {edge.from} → {edge.to}
                  </div>
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="demo-card__actions">
        <button className="clone-link-btn" type="button" onClick={() => onFeedback(item.item_id, "click")}>
          click
        </button>
        <button className="clone-link-btn" type="button" onClick={() => onFeedback(item.item_id, "purchase")}>
          purchase
        </button>
      </div>
    </article>
  );
}
