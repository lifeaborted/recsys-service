import { formatPrice } from "../lib/format";
import type { Recommendation } from "../api/recsysTypes";

interface Props {
  loading: boolean;
  error: string | null;
  items?: Recommendation[];
  empty?: string;
  onFavorite?: () => void;
  onPurchase?: (itemId: string) => void;
  busy?: boolean;
}

export function PortalRecommendationShelf({ loading, error, items, empty, onFavorite, onPurchase, busy }: Props) {
  if (error) return <p role="alert" className="portal-shelf-status">{error}</p>;
  if (loading) return <p role="status" className="portal-shelf-status">Подбираем товары…</p>;
  if (!items?.length) return <p className="portal-shelf-status">{empty ?? "Для этого профиля пока нет рекомендаций."}</p>;

  return (
    <div className="portal-shelf">
      {items.map((item) => {
        const href = onPurchase ? `#/demo-product?item=${encodeURIComponent(item.item_id)}` : undefined;
        return (
          <article className="portal-suggestion" key={item.item_id}>
            {onFavorite && <button type="button" className="portal-suggestion__favorite" aria-label="Добавить в избранное" onClick={onFavorite}>
              <svg aria-hidden="true"><use href="/assets/store/icons.svg#heart" /></svg>
            </button>}
            <div className="portal-suggestion__image" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.2">
                <path strokeLinejoin="round" d="M12 2.7 21 8v8l-9 5.3L3 16V8l9-5.3ZM3 8l9 5.3L21 8m-9 5.3v8M7.5 5.3l9 5.3v4.1" />
              </svg>
            </div>
            {item.category && <span className="portal-suggestion__sku">{item.category}</span>}
            <h3>{href ? <a href={href}>{item.title}</a> : item.title}</h3>
            {item.price != null && <strong>{formatPrice(item.price, item.currency_code)}</strong>}
            {item.explanation && <details>
              <summary>Почему рекомендуем</summary>
              <p>{item.explanation}</p>
            </details>}
            {onPurchase && <div className="portal-suggestion__actions">
              <a href={href}>Подробнее</a>
              <button className="button button_primary button_size-l" type="button" disabled={busy} onClick={() => onPurchase(item.item_id)}>Купить товар</button>
            </div>}
          </article>
        );
      })}
    </div>
  );
}
