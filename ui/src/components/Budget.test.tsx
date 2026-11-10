import React from 'react';
import { render, renderHook, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useTokens } from '../theme/tokens';
import { readBudget } from '../utils/budget';
import { BudgetGauge } from './Budget';

describe('BudgetGauge', () => {
  it.each([
    [300, 'Within budget', 'accent', 'success'],
    [400, 'Getting close', 'warning', 'warning'],
    [480, 'Nearly at budget', 'danger', 'danger'],
    [600, 'Over budget', 'danger', 'danger'],
  ] as const)('colours $%d of $500 as %s', (cost, label, fill, icon) => {
    const t = renderHook(() => useTokens()).result.current;
    const budget = readBudget('500', cost);
    if (!budget) throw new Error('expected a budget');

    const { container } = render(<BudgetGauge budget={budget} />);

    const expected = document.createElement('span');
    expected.style.backgroundColor = t[fill];
    const bar = screen.getByRole('meter').firstElementChild as HTMLElement;
    expect(bar.style.backgroundColor).toBe(expected.style.backgroundColor);
    expect(container.querySelector('svg')?.firstElementChild?.getAttribute('fill')).toBe(t[icon]);
    expect(container.textContent).toContain(label);
  });
});
