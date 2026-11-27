import type { MouseEvent } from "react";
import type { RecommendationItem } from "../api/types";
import { findProduct } from "../data/catalog";
import { formatPrice } from "../lib/format";

interface RecommendationCardProps {
  item: RecommendationItem;
  showScores?: boolean;
  onFeedback?: (item: RecommendationItem) => void;
}

/** Карточка «Вам может подойти» с разметкой и классами исходного сайта. */
export function RecommendationCard({ item, showScores, onFeedback }: RecommendationCardProps) {
  const product = findProduct(item.item_id);
  const image = item.image_url || product?.image || "/assets/scale-dec-168x168.png";
  const synthetic = item.item_id.startsWith("DEMO-");
  const url = synthetic ? "#/demo-product?item=" + encodeURIComponent(item.item_id) : item.url || product?.url || "#";
  const vendor = item.subtitle || product?.vendor || item.category || "";
  const price = item.price ?? product?.price ?? 0;

  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    onFeedback?.(item);
    if (url === "#") event.preventDefault();
  };

  return (
    <article className="suggestion" data-v-b629fb3c="">
      {showScores ? (
        <span className="suggestion__meta">{Math.round(item.score * 100)}%</span>
      ) : null}
      <a
        className="suggestion__image-box"
        data-v-b629fb3c=""
        href={url}
        target={synthetic ? undefined : "_blank"}
        rel="noreferrer"
        onClick={handleClick}
      >
        <img className="suggestion__image" data-v-b629fb3c="" src={image} alt="Product" />
      </a>
      <span className="suggestion__vendor text-size-md sl-color-text-secondary" data-v-b629fb3c="">
        {vendor}
      </span>
      <span className="suggestion__title text-size-md sl-color-text" data-v-b629fb3c="">
        <a
          className="suggestion__description"
          data-v-b629fb3c=""
          href={url}
          target={synthetic ? undefined : "_blank"}
          rel="noreferrer"
          onClick={handleClick}
        >
          {item.title}
        </a>
      </span>
      {showScores && item.reason ? (
        <span className="suggestion__reason">{item.reason}</span>
      ) : null}
      <div className="suggestion__aligner" data-v-b629fb3c="" />
      <div className="suggestion__purchase" data-v-b629fb3c="">
        <div className="suggestion__purchase-row" data-v-b629fb3c="">
          <div className="suggestion__purchase-info" data-v-b629fb3c="">
            <p className="suggestion__old-price" data-v-b629fb3c="" />
            <span className="heading-xl sl-color-text" data-v-b629fb3c="">
              {formatPrice(price, item.currency)}
            </span>
          </div>
        </div>
      </div>
      <a
        className="button button_primary button_size-l button_block link-reset mt-12"
        data-v-b629fb3c=""
        href={url}
        target={synthetic ? undefined : "_blank"}
        rel="noreferrer"
        onClick={handleClick}
      >
        <span className="button__text">Посмотреть</span>
      </a>
    </article>
  );
}
