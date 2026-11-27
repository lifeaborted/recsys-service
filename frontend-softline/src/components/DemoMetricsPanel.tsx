import { demoMetrics as metrics } from "../data/demoMetrics";
import { DemoQualityChart } from "./DemoQualityChart";

function percent(value: number): string {
  return `${value.toFixed(2).replace(".", ",")}%`;
}

function share(value: number): string {
  return `${Number(value.toFixed(2)).toString().replace(".", ",")}%`;
}

export function DemoMetricsPanel({
  feedDate,
  catalogVersion,
}: { feedDate?: string; catalogVersion?: string }) {
  const variants = metrics.variant_hits;
  const visits = metrics.visits_without_target_action;

  return (
    <section className="demo-quality" aria-labelledby="demo-quality-title">
      <div className="demo-quality__head">
        <h2 id="demo-quality-title">Качество против baseline</h2>
      </div>

      <div className="demo-quality__scroll">
        <table className="demo-quality__table">
          <thead>
            <tr>
              <th rowSpan={2} scope="col">Метод</th>
              <th colSpan={3} scope="colgroup">Когорта A</th>
              <th colSpan={3} scope="colgroup">Когорта B</th>
            </tr>
            <tr>
              <th scope="col">все</th>
              <th scope="col">B2B</th>
              <th scope="col">B2C</th>
              <th scope="col">все</th>
              <th scope="col">B2B</th>
              <th scope="col">B2C</th>
            </tr>
          </thead>
          <tbody>
            {metrics.precision_at_3.map((row) => (
              <tr key={row.method}>
                <th scope="row">{row.label}</th>
                <td>{percent(row.A.all)}</td>
                <td>{percent(row.A.B2B)}</td>
                <td>{percent(row.A.B2C)}</td>
                <td>{percent(row.B.all)}</td>
                <td>{percent(row.B.B2B)}</td>
                <td>{percent(row.B.B2C)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className="demo-quality__line">
        Варианты гибрида: A {variants.A.hits} из {variants.A.total} ({share(variants.A.share)}),
        B {variants.B.hits} из {variants.B.total} ({share(variants.B.share)}).
      </p>
      <p className="demo-quality__line demo-quality__line_note">
        Около {visits.share}% визитов уходят без целевого действия. {visits.note}
      </p>
      <p className="demo-quality__line">
        Baseline: история продаж до {metrics.baseline_cutoff.split("-").reverse().join(".")}.
        {" "}{metrics.protocol}
      </p>
      <p className="demo-quality__line demo-quality__line_feed">
        Дата фида: <strong>{feedDate?.slice(0, 10) ?? metrics.feed_date}</strong> ({catalogVersion ?? (feedDate ? "каталог API" : metrics.feed_label)}). {metrics.catalog_note}
      </p>
      <p className="demo-quality__line demo-quality__line_note">{metrics.feed_note}</p>
      <DemoQualityChart />
    </section>
  );
}
