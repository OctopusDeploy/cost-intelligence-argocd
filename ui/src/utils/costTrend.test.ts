import { describe, expect, it } from 'vitest';
import { DailyCost } from './api';
import { summarizeCostTrend, trendLevel } from './costTrend';

const NOW = new Date('2026-11-17T12:00:00Z');

// series gives one day per entry, ending the day before NOW.
const series = (costs: number[]): DailyCost[] =>
  costs.map((totalCost, i) => {
    const date = new Date(NOW.getTime() - (costs.length - i) * 86_400_000).toISOString().slice(0, 10);
    return { date, totalCost, cpuCost: 0, memoryCost: 0, networkCost: 0, storageCost: 0 };
  });

describe('summarizeCostTrend', () => {
  it('has no trend with less than two weeks of complete days', () => {
    expect(summarizeCostTrend(series(Array(13).fill(10)), 30, NOW)).toBeNull();
  });

  // The first day of a series that starts inside the window is partial, so it is left out.
  it('compares the last 7 days with the 7 before', () => {
    const summary = summarizeCostTrend(series([1, ...Array(7).fill(10), ...Array(7).fill(13)]), 30, NOW);
    expect(summary?.change).toBeCloseTo(0.3);
    expect(summary?.level).toBe('critical');
  });

  it('reports a falling cost as ok', () => {
    const summary = summarizeCostTrend(series([1, ...Array(7).fill(10), ...Array(7).fill(8)]), 30, NOW);
    expect(summary?.change).toBeCloseTo(-0.2);
    expect(summary?.level).toBe('ok');
  });
});

describe('trendLevel', () => {
  it('is green up to 5%, yellow up to 20% and red above', () => {
    expect(trendLevel(0.05)).toBe('ok');
    expect(trendLevel(0.06)).toBe('warning');
    expect(trendLevel(0.2)).toBe('warning');
    expect(trendLevel(0.21)).toBe('critical');
  });
});
