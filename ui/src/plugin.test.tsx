import React from 'react';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApplicationDetailsTabProps, CostIntelligenceTab } from './plugin';
import { BUDGET_ANNOTATION } from './utils/budget';

vi.mock('./components/CostPanel', () => ({
  CostPanel: ({ budgetAnnotation }: { budgetAnnotation?: string }) => `cost views, budget ${budgetAnnotation ?? 'none'}`,
}));

function application(destination: NonNullable<ApplicationDetailsTabProps['application']['spec']>['destination']) {
  return { metadata: { name: 'web', namespace: 'argocd' }, spec: { destination } };
}

describe('CostIntelligenceTab', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the cost views of an application that deploys to this cluster', () => {
    render(<CostIntelligenceTab application={application({ server: 'https://kubernetes.default.svc', namespace: 'shop' })} />);

    expect(document.body.textContent).toContain('cost views');
  });

  it('passes on the budget annotation of the application', () => {
    const app = application({ server: 'https://kubernetes.default.svc', namespace: 'shop' });
    render(
      <CostIntelligenceTab
        application={{ ...app, metadata: { ...app.metadata, annotations: { [BUDGET_ANNOTATION]: '500', other: '7' } } }}
      />
    );

    expect(document.body.textContent).toContain('budget 500');
  });

  it('passes on no budget when the application has no annotations', () => {
    render(<CostIntelligenceTab application={application({ server: 'https://kubernetes.default.svc', namespace: 'shop' })} />);

    expect(document.body.textContent).toContain('budget none');
  });

  it('explains, without asking for costs, that an application on another cluster is not measured', () => {
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);

    render(<CostIntelligenceTab application={application({ name: 'prod-eu', server: 'https://10.4.0.12:6443', namespace: 'shop' })} />);

    expect(screen.getByRole('heading', { name: 'Deploys to another cluster' })).not.toBeNull();
    expect(document.body.textContent).toContain('prod-eu');
    expect(screen.getByRole('banner').textContent).toContain('Not measured');
    expect(document.body.textContent).not.toContain('cost views');
    expect(screen.queryByRole('tablist')).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it('names another cluster by its server when it has no name', () => {
    render(<CostIntelligenceTab application={application({ server: 'https://10.4.0.12:6443', namespace: 'shop' })} />);

    expect(document.body.textContent).toContain('https://10.4.0.12:6443');
  });

  it('links the single-cluster limit in a new tab', () => {
    render(<CostIntelligenceTab application={application({ name: 'prod-eu' })} />);

    const link = screen.getByRole('link', { name: 'Single-cluster limit (opens in a new tab)' });
    expect(link.getAttribute('href')).toBe(
      'https://cost-intelligence-argocd.readthedocs.io/en/stable/how-costs-are-measured/#applications-on-another-cluster',
    );
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });
});
