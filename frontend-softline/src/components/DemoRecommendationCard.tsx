import { useState } from "react";
import type { GraphEdge, Recommendation, Variant } from "../api/recsysTypes";

interface DemoRecommendationCardProps {
  item: Recommendation;
  edges: GraphEdge[];
  onPurchase?: (itemId: string) => void;
  onShowGraph?: () => void;
  busy: boolean;
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

function evidenceURL(edge: GraphEdge): string | null {
  if (edge.type !== "upgrade" && edge.type !== "complement" && edge.type !== "renewal") {
    return null;
  }
  const reference = edge.source.replace(/@[^@/#?\s]+#[^@#\s]+$/, "");
  try {
    const url = new URL(reference);
    return (url.protocol === "https:" || url.protocol === "http:") && !url.username && !url.password
      ? url.href
      : null;
  } catch {
    return null;
  }
}

export function DemoRecommendationCard({ item, edges, onPurchase, onShowGraph, busy }: DemoRecommendationCardProps) {
  const [showVariants, setShowVariants] = useState(false);
  const [showReason, setShowReason] = useState(false);

  const verification = item.status === "review"
    ? { label: "требует проверки", className: "demo-card__verification demo-card__verification_check" }
    : item.status === "confirmed"
      ? { label: "подтверждено", className: "demo-card__verification demo-card__verification_confirmed" }
      : null;

  return (
    <article className={`demo-card${item.status === "review" ? " demo-card_requires-check" : ""}`}>
      <div className="demo-card__badges">
        <span className="demo-card__kind">{KIND_LABELS[item.recommendation_kind]}</span>
        {verification ? <span className={verification.className}>{verification.label}</span> : null}
      </div>
      <h3 className="demo-card__title">{item.title}</h3>
      <p className="demo-card__explanation">{item.explanation}</p>
      {item.url ? (
        <a className="demo-card__link" href={item.url} target="_blank" rel="noreferrer">
          Открыть карточку →
        </a>
      ) : (
        <span className="demo-card__link">Ссылка недоступна</span>
      )}

      {onPurchase ? <a
        className="demo-card__detail-link"
        href={`#/demo-product?item=${encodeURIComponent(item.item_id)}`}
      >
        Полная информация о товаре →
      </a> : null}

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
                    {onPurchase ? <button className="clone-link-btn" type="button" disabled={busy} onClick={() => onPurchase(variant.item_id)}>Купить {variant.item_id}</button> : null}
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
          <div className="demo-card__explain-actions">
            {onShowGraph ? <button className="clone-link-btn clone-link-btn_primary" type="button" onClick={onShowGraph}>
              Показать граф
            </button> : null}
            <button className="clone-link-btn" type="button" onClick={() => setShowReason((value) => !value)}>
              {showReason ? "Скрыть причину" : "Почему рекомендуем"}
            </button>
          </div>
          {showReason ? (
            <div style={{ marginTop: 10 }}>
              {edges.map((edge) => {
                const url = evidenceURL(edge);
                return (
                  <div key={edge.id}>
                    <p className="demo-card__reason">{edge.reason}</p>
                    {onShowGraph ? <div className="demo-card__source">
                      {edge.type} · {edge.source} · {edge.from} → {edge.to}
                      {url ? (
                        <> · <a className="demo-card__link" href={url} target="_blank" rel="noreferrer">Открыть источник</a></>
                      ) : null}
                    </div> : null}
                  </div>
                );
              })}
            </div>
          ) : null}
        </div>
      ) : null}

      {onPurchase ? <div className="demo-card__actions">
        <button className="clone-link-btn" type="button" disabled={busy} onClick={() => onPurchase(item.item_id)}>
          Купить товар
        </button>
      </div> : null}
    </article>
  );
}
