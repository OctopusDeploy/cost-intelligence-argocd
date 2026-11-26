import React from 'react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AppCostStatus } from './AppCostStatus';

const costData = vi.hoisted(() => ({ current: null as unknown }));
vi.mock('../hooks/useCostData', () => ({ useCostData: () => ({ data: costData.current }) }));
vi.mock('../hooks/useMeasuredClusters', () => ({ useMeasuredClusters: () => [] }));

const application = { metadata: { name: 'web', namespace: 'argocd' }, spec: { project: 'default' } };

describe('AppCostStatus', () => {
  it('shows the monthly cost with its idle part as detail, not as an addend', () => {
    costData.current = {
      scope: 'application',
      totalMonthlyCost: 100,
      efficiency: { cpuIdleCost: 20, ramIdleCost: 5 },
      cloudEstimate: { totalMonthlyCost: 10 },
    };
    render(<AppCostStatus application={application} />);

    expect(screen.getByText('$110.00')).toBeTruthy();
    expect(screen.getByText('Kubernetes $100.00 + cloud $10.00, incl. $25.00 idle')).toBeTruthy();
  });

  it('shows nothing before there is a measurement', () => {
    costData.current = null;
    const { container } = render(<AppCostStatus application={application} />);

    expect(container.textContent).toBe('');
  });
});
