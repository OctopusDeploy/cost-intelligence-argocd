import { formatPercent } from './format';

// BUDGET_ANNOTATION on an Application holds its monthly budget, in dollars.
export const BUDGET_ANNOTATION = 'cost-intelligence.octopus.com/monthly-budget';

export type BudgetLevel = 'within' | 'close' | 'nearly' | 'over';

export type Budget =
  | { kind: 'unreadable'; value: string }
  | { kind: 'set'; amount: number; cost: number; fraction: number; share: string; level: BudgetLevel };

const CLOSE_ABOVE = 75;
const NEARLY_ABOVE = 90;
const OVER_ABOVE = 100;

// At most this much of a value that can't be read is shown: an annotation can hold 256 KiB.
const MAX_SHOWN_VALUE = 60;

// A plain amount only. Number and parseFloat would also take "1e3" and "0x1F4", or read "500 USD" as 500.
const AMOUNT = /^\d+(\.\d{1,2})?$/;

// readBudget compares an application's projected monthly cost with the budget in its annotation. It
// returns null when the application has no annotation, so that nothing is shown for it.
export function readBudget(annotation: string | undefined, cost: number): Budget | null {
  if (annotation === undefined) return null;
  const value = annotation.trim();
  const amount = AMOUNT.test(value) ? Number(value) : 0;
  // Number gives Infinity for a run of digits too long to hold.
  if (!(amount > 0 && Number.isFinite(amount))) {
    return { kind: 'unreadable', value: value.length > MAX_SHOWN_VALUE ? `${value.slice(0, MAX_SHOWN_VALUE)}…` : value };
  }
  return { kind: 'set', amount, cost, fraction: cost / amount, share: shareLabel(cost, amount), level: level(cost, amount) };
}

// above compares in whole cents: $260.10 of $289 is exactly 90%, but the division gives 90.00000000000001.
function above(cost: number, amount: number, percent: number): boolean {
  return Math.round(cost * 100) * 100 > percent * Math.round(amount * 100);
}

function level(cost: number, amount: number): BudgetLevel {
  if (above(cost, amount, OVER_ABOVE)) return 'over';
  if (above(cost, amount, NEARLY_ABOVE)) return 'nearly';
  return above(cost, amount, CLOSE_ABOVE) ? 'close' : 'within';
}

// shareLabel keeps one decimal just past a threshold, so "100%" is never shown beside "Over budget".
function shareLabel(cost: number, amount: number): string {
  const fraction = cost / amount;
  const percent = fraction * 100;
  const rounded = Math.round(percent);
  const pastThreshold = [CLOSE_ABOVE, NEARLY_ABOVE, OVER_ABOVE].includes(rounded) && above(cost, amount, rounded);
  return pastThreshold ? `${percent.toFixed(1)}%` : formatPercent(fraction);
}
