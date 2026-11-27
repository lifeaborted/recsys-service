import { useEffect } from "react";
import type { ScoreHistory } from "../api/types";
import { formatDateTime } from "../lib/format";

interface DemoScoreModalProps {
  history: ScoreHistory;
  onClose: () => void;
}

const W = 680;
const H = 240;
const PAD_L = 46;
const PAD_R = 16;
const PAD_T = 18;
const PAD_B = 34;

export function DemoScoreModal({ history, onClose }: DemoScoreModalProps) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const previous = document.body.style.overflow;
    document.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = previous;
    };
  }, [onClose]);

  const points = history.points;
  const times = points.map((point) => new Date(point.at).getTime()).filter(Number.isFinite);
  const minTime = times.length ? Math.min(...times) : 0;
  const maxTime = times.length ? Math.max(...times) : 0;
  const span = maxTime - minTime || 1;
  const x = (at: string) => PAD_L + ((new Date(at).getTime() - minTime) / span) * (W - PAD_L - PAD_R);
  const y = (score: number) => PAD_T + (1 - Math.min(1, Math.max(0, score))) * (H - PAD_T - PAD_B);
  const line = points.map((point) => `${x(point.at)},${y(point.score)}`).join(" ");
  const totalRuns = points.reduce((sum, point) => sum + point.runs, 0);

  return (
    <div
      className="demo-modal"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="demo-modal__window" role="dialog" aria-modal="true" aria-labelledby="score-modal-title">
        <div className="demo-modal__head">
          <div>
            <h2 id="score-modal-title">График score товара</h2>
            <p className="demo-modal__subtitle">
              {history.item_id} · средний score у всех пользователей: {history.average.toFixed(3)}
            </p>
          </div>
          <button className="demo-modal__close" type="button" aria-label="Закрыть график" onClick={onClose}>×</button>
        </div>

        {points.length === 0 ? (
          <div className="reco-empty" style={{ marginTop: 16 }}>По этому товару пока нет сохранённых выдач.</div>
        ) : (
          <>
            <svg className="score-chart" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="График среднего score">
              {[0, 0.5, 1].map((value) => (
                <g key={value}>
                  <line x1={PAD_L} y1={y(value)} x2={W - PAD_R} y2={y(value)} className="score-chart__grid" />
                  <text x={PAD_L - 8} y={y(value) + 4} textAnchor="end" className="score-chart__axis">{value.toFixed(1)}</text>
                </g>
              ))}
              <line
                x1={PAD_L}
                y1={y(history.average)}
                x2={W - PAD_R}
                y2={y(history.average)}
                className="score-chart__average"
              />
              <text x={W - PAD_R} y={y(history.average) - 6} textAnchor="end" className="score-chart__legend">
                среднее {history.average.toFixed(3)}
              </text>
              {points.length > 1 ? (
                <polyline points={line} className="score-chart__line" fill="none" />
              ) : null}
              {points.map((point, index) => (
                <g key={`${point.at}-${index}`}>
                  <circle cx={x(point.at)} cy={y(point.score)} r="4" className="score-chart__dot" />
                  <title>{`${formatDateTime(point.at)} · ${point.score.toFixed(3)} · выдач: ${point.runs}`}</title>
                </g>
              ))}
              <text x={PAD_L} y={H - 10} className="score-chart__axis">{formatDateTime(points[0].at)}</text>
              <text x={W - PAD_R} y={H - 10} textAnchor="end" className="score-chart__axis">
                {formatDateTime(points[points.length - 1].at)}
              </text>
            </svg>
            <p className="demo-chart__note">
              Точек: {points.length} · выдач: {totalRuns} · данные из всех сохранённых run'ов стенда.
            </p>
          </>
        )}
      </div>
    </div>
  );
}
