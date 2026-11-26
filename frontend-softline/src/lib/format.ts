const numberFormat = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 });

const dateTimeFormat = new Intl.DateTimeFormat("ru-RU", {
  day: "2-digit",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
});

export function formatPrice(value: number | undefined, currency = "RUB"): string {
  if (typeof value !== "number" || Number.isNaN(value)) return "—";
  const suffix = currency === "RUB" || currency === "₽" ? "₽" : currency;
  return `${numberFormat.format(value)} ${suffix}`;
}

export function formatScore(score: number): string {
  return `${Math.round(score * 100)}%`;
}

export function formatDateTime(iso: string | undefined): string {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return dateTimeFormat.format(date);
}

export function formatDuration(ms: number | null | undefined): string {
  if (typeof ms !== "number") return "—";
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)} с` : `${ms} мс`;
}

export function plural(count: number, forms: [string, string, string]): string {
  const mod10 = count % 10;
  const mod100 = count % 100;
  if (mod10 === 1 && mod100 !== 11) return forms[0];
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms[1];
  return forms[2];
}
