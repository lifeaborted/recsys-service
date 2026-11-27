import fallback from "../../../recsys/docs/demo/metrics.json";
import published from "../../../recsys/tests/e2e/data/public-metrics.json";

const methodKeys = {
  popularity: "popularity",
  co_purchase: "copurchase",
  hybrid: "hybrid",
} as const;
const cohorts = ["A", "B"] as const;
const groups = ["all", "B2B", "B2C"] as const;

function cohortMetrics(cohort: typeof cohorts[number], method: keyof typeof methodKeys, local: Record<typeof groups[number], number>) {
  const precision = (group: typeof groups[number]) =>
    published.cells.find((cell) => cell.cohort === cohort && cell.group === group)?.[methodKeys[method]].precision_at_3
      ?? local[group];
  return { all: precision("all"), B2B: precision("B2B"), B2C: precision("B2C") };
}

function hybridVariants(cohort: typeof cohorts[number]) {
  const hybrid = published.cells.find((cell) => cell.cohort === cohort && cell.group === "all")?.hybrid;
  if (hybrid?.variant_hits == null || hybrid.hits == null || hybrid.variant_share == null) {
    return fallback.variant_hits[cohort];
  }
  return { hits: hybrid.variant_hits, total: hybrid.hits, share: hybrid.variant_share };
}

export const demoMetrics = {
  ...fallback,
  hybridCoverage: published.cells.map(({ cohort, group, hybrid }) => ({ cohort, group, ...hybrid })),
  baseline_cutoff: published.train_through,
  precision_at_3: fallback.precision_at_3.map((row) => {
    const method = row.method as keyof typeof methodKeys;
    return {
      ...row,
      A: cohortMetrics("A", method, row.A),
      B: cohortMetrics("B", method, row.B),
    };
  }),
  variant_hits: { A: hybridVariants("A"), B: hybridVariants("B") },
};
