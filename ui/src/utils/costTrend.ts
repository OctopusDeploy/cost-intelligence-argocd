import { DailyCost } from './api';
import { completeDays } from './ranges';

const WEEK = 7;
const DAY_MS = 86_400_000;

export type TrendLevel = 'ok' | 'warning' | 'critical';

export interface CostTrendSummary {
  // change is the relative change of the daily cost: the last 7 complete days against the 7 before.
  change: number;
  level: TrendLevel;
}

// A rise up to WARNING_RISE is noise (a busy weekend, a rolled-out pod); above CRITICAL_RISE the
// month's cost is heading somewhere the owner should look at.
export const WARNING_RISE = 0.05;
export const CRITICAL_RISE = 0.2;

export function trendLevel(change: number): TrendLevel {
  if (change > CRITICAL_RISE) return 'critical';
  if (change > WARNING_RISE) return 'warning';
  return 'ok';
}

// summarizeCostTrend compares the average daily cost of the last 7 complete days with the 7 before.
// `days` is the series asked for over the last `window` days ending today. It returns null with fewer
// than 14 complete days, or when the earlier week cost nothing, so no percentage can be given.
export function summarizeCostTrend(days: DailyCost[], window: number, now: Date = new Date()): CostTrendSummary | null {
  const today = now.toISOString().slice(0, 10);
  const from = new Date(now.getTime() - (window - 1) * DAY_MS).toISOString().slice(0, 10);
  const complete = completeDays(days, today, from);
  if (complete.length < 2 * WEEK) return null;

  const average = (series: DailyCost[]) => series.reduce((sum, day) => sum + day.totalCost, 0) / series.length;
  const recent = average(complete.slice(-WEEK));
  const earlier = average(complete.slice(-2 * WEEK, -WEEK));
  if (earlier <= 0) return null;

  const change = recent / earlier - 1;
  return { change, level: trendLevel(change) };
}
