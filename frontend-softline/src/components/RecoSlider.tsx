import type { MouseEvent } from "react";
import type { RecommendationItem } from "../api/types";
import type { RecommendationsState } from "../hooks/useRecommendations";
import { RecommendationCard } from "./RecommendationCard";

interface RecoSliderProps {
  state: RecommendationsState;
  showScores?: boolean;
  onFeedback?: (item: RecommendationItem) => void;
}

function chevron(direction: "prev" | "next") {
  const path = direction === "prev" ? "M14 18L8 12L14 6" : "M9 18L15 12L9 6";
  return (
    <svg className="icon icon-size-20 sl-color-icon button__icon" viewBox="0 0 24 24" aria-hidden="true">
      <path
        d={path}
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function RecoSlider({ state, showScores, onFeedback }: RecoSliderProps) {
  const items = state.data?.items ?? [];

  if (state.status === "error") {
    return <div className="reco-empty">Не удалось получить рекомендации: {state.error}</div>;
  }
  if (state.status === "loading" && !items.length) {
    return <div className="reco-empty">Загружаем рекомендации…</div>;
  }
  if (!items.length) {
    return <div className="reco-empty">Рекомендаций пока нет</div>;
  }

  const next = (event: MouseEvent<HTMLButtonElement>, direction: number) => {
    const slider = event.currentTarget.closest<HTMLElement>(".slider");
    const swiper = slider?.querySelector<HTMLElement>(".swiper");
    const slide = swiper?.querySelector<HTMLElement>(".swiper-slide");
    if (!swiper) return;
    const width = slide ? slide.getBoundingClientRect().width + 24 : swiper.clientWidth * 0.8;
    swiper.scrollBy({ left: direction * width, behavior: "smooth" });
  };

  return (
    <div className="slider" data-v-af5a7bd7="" id="suggestion-widget">
      <div className="slider__control slider__control_prev" data-v-af5a7bd7="">
        <button
          className="button button_default button_size-l button_circle"
          type="button"
          onClick={(event) => next(event, -1)}
        >
          {chevron("prev")}
        </button>
      </div>
      <div
        className="swiper swiper-initialized swiper-horizontal slider__swiper swiper-backface-hidden"
        data-v-af5a7bd7=""
      >
        <div className="swiper-wrapper">
          {items.map((item, index) => (
            <div
              key={`${item.item_id}-${index}`}
              className="swiper-slide width-auto"
              data-v-af5a7bd7=""
              style={{ marginRight: "24px" }}
              data-swiper-slide-index={index}
            >
              <RecommendationCard item={item} showScores={showScores} onFeedback={onFeedback} />
            </div>
          ))}
        </div>
      </div>
      <div className="slider__control slider__control_next" data-v-af5a7bd7="">
        <button
          className="button button_default button_size-l button_circle"
          type="button"
          onClick={(event) => next(event, 1)}
        >
          {chevron("next")}
        </button>
      </div>
    </div>
  );
}
