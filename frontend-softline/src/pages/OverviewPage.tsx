import { Fragment } from "../components/Fragment";
import { RecoSlider } from "../components/RecoSlider";
import { EVENTS_HTML, INFO_WIDGETS_HTML, POPULAR_HTML, PROMO_HTML } from "../data/chrome";
import { useCloneRecommendations } from "../hooks/useCloneRecommendations";
import { useDebug } from "../state/debug";
import { useSettings } from "../state/settings";

export function OverviewPage() {
  const reco = useCloneRecommendations();
  const { showScores } = useDebug();
  const { settings, update } = useSettings();

  return (
    <div className="home-page">
      <div className="mobile-hidden">
        <Fragment html={INFO_WIDGETS_HTML} />
      </div>
      <Fragment html={PROMO_HTML} />

      <div className="flex-column gap-16">
        <div className="reco-heading-row">
          <span className="heading-xl sl-color-text">Вам может подойти</span>
          <label className="reco-limit">
            Показать{" "}
            <select
              aria-label="Количество рекомендаций"
              value={settings.limit}
              onChange={(event) => update({ limit: Number(event.target.value) })}
            >
              {[1, 2, 3, 4, 5, 6, 9, 12, 24, 36].map((value) => (
                <option key={value} value={value}>{value}</option>
              ))}
            </select>
          </label>
          <span className="reco-chip">
            {settings.mockMode ? "mock" : "live"} · {settings.scenario} ·{" "}
            {reco.data?.items.length ?? 0} товаров
            {reco.meta ? ` · ${reco.meta.durationMs} мс` : ""}
          </span>
        </div>
        <RecoSlider state={reco} showScores={showScores} onFeedback={reco.onFeedback} />
      </div>

      <div className="flex-column gap-16" style={{ marginTop: 32 }}>
        <span className="heading-xl sl-color-text">Популярные решения</span>
        <Fragment html={POPULAR_HTML} />
      </div>

      <div className="flex-column gap-16" style={{ marginTop: 32 }}>
        <span className="heading-xl sl-color-text">Ближайшие мероприятия</span>
        <Fragment html={EVENTS_HTML} />
      </div>
    </div>
  );
}
