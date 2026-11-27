import { demoMetrics as metrics } from "../data/demoMetrics";

type Cohort = "all" | "B2B" | "B2C";

const COHORTS: Array<{ key: Cohort; label: string }> = [
  { key: "all", label: "все" },
  { key: "B2B", label: "B2B" },
  { key: "B2C", label: "B2C" },
];

const SCALE_MAX = 22;

function percent(value: number): string {
  return `${Number(value.toFixed(2)).toString().replace(".", ",")}%`;
}

/**
 * Наглядное сравнение A и B по тем же числам, что и таблица выше.
 * Без внешних библиотек: обычные полосы шириной от значения.
 */
export function DemoQualityChart() {
  return (
    <section className="demo-chart" aria-labelledby="demo-chart-title">
      <div className="demo-chart__head">
        <h3 id="demo-chart-title">Precision@3: A против B</h3>
        <div className="demo-chart__legend">
          <span><i className="demo-chart__swatch demo-chart__swatch_a" />Когорта A</span>
          <span><i className="demo-chart__swatch demo-chart__swatch_b" />Когорта B</span>
        </div>
      </div>

      <div className="demo-chart__body">
        {metrics.precision_at_3.map((row) => (
          <div className="demo-chart__group" key={row.method}>
            <h4>{row.label}</h4>
            {COHORTS.map((cohort) => (
              <div className="demo-chart__row" key={cohort.key}>
                <span className="demo-chart__cohort">{cohort.label}</span>
                <div className="demo-chart__bars">
                  {(["A", "B"] as const).map((variant) => {
                    const value = row[variant][cohort.key];
                    return (
                      <div
                        className={`demo-chart__bar demo-chart__bar_${variant.toLowerCase()}`}
                        style={{ width: `${(value / SCALE_MAX) * 100}%` }}
                        key={variant}
                      >
                        <span>{variant} · {percent(value)}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        ))}
      </div>

      <p className="demo-chart__note">
        Шкала до {SCALE_MAX}%. Около {metrics.visits_without_target_action.share}% визитов без целевого
        действия в график не входят: это не Precision@3.
      </p>
    </section>
  );
}
