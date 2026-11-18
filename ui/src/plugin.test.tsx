import React from 'react';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApplicationDetailsTabProps, CostIntelligenceTab } from './plugin';
import api from './utils/api';

vi.mock('./components/CostPanel', () => ({ CostPanel: () => 'cost views' }));
vi.mock('./utils/api', () => ({ default: { getMeasuredClusters: vi.fn(), getKnownClusters: vi.fn() } }));

const getMeasuredClusters = vi.mocked(api.getMeasuredClusters);
const getKnownClusters = vi.mocked(api.getKnownClusters);

function application(destination: NonNullable<ApplicationDetailsTabProps['application']['spec']>['destination']) {
  return { metadata: { name: 'web', namespace: 'argocd' }, spec: { destination } };
}

describe('CostIntelligenceTab', () => {
  beforeEach(() => {
    getMeasuredClusters.mockReset();
    getMeasuredClusters.mockResolvedValue([]);
    getKnownClusters.mockResolvedValue([]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the cost views of an application that deploys to this cluster', () => {
    render(<CostIntelligenceTab application={application({ server: 'https://kubernetes.default.svc', namespace: 'shop' })} />);

    expect(document.body.textContent).toContain('cost views');
    expect(getMeasuredClusters).not.toHaveBeenCalled();
  });

  it('explains, without asking for costs, that an application on another cluster is not measured', async () => {
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);

    render(<CostIntelligenceTab application={application({ name: 'prod-eu', server: 'https://10.4.0.12:6443', namespace: 'shop' })} />);

    expect(await screen.findByRole('heading', { name: 'Deploys to another cluster' })).not.toBeNull();
    expect(document.body.textContent).toContain('prod-eu');
    expect(screen.getByRole('banner').textContent).toContain('Not measured');
    expect(document.body.textContent).not.toContain('cost views');
    expect(screen.queryByRole('tablist')).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it('names another cluster by its server when it has no name', async () => {
    render(<CostIntelligenceTab application={application({ server: 'https://10.4.0.12:6443', namespace: 'shop' })} />);

    await screen.findByRole('heading', { name: 'Deploys to another cluster' });
    expect(document.body.textContent).toContain('https://10.4.0.12:6443');
  });

  it('shows the cost views of an application on a cluster the service measures', async () => {
    getMeasuredClusters.mockResolvedValue([{ name: 'CICD' }]);

    render(<CostIntelligenceTab application={application({ name: 'CICD', namespace: 'shop' })} />);

    expect(await screen.findByText('cost views')).not.toBeNull();
    expect(screen.queryByRole('heading', { name: 'Deploys to another cluster' })).toBeNull();
  });

  it('lists the clusters Kubecost knows for an application on one that is not matched', async () => {
    getKnownClusters.mockResolvedValue(['eks-cicd', 'eks-eu']);

    render(<CostIntelligenceTab application={application({ name: 'CICD', namespace: 'shop' })} />);

    await screen.findByRole('heading', { name: 'Deploys to another cluster' });
    expect(await screen.findByText(/Kubecost has cost for: eks-cicd, eks-eu/)).not.toBeNull();
  });

  it('shows nothing while it asks which clusters are measured', () => {
    getMeasuredClusters.mockReturnValue(new Promise(() => {}));

    const { container } = render(<CostIntelligenceTab application={application({ name: 'CICD' })} />);

    expect(container.textContent).toBe('');
  });

  it('links the single-cluster limit in a new tab', async () => {
    render(<CostIntelligenceTab application={application({ name: 'prod-eu' })} />);

    const link = await screen.findByRole('link', { name: 'Single-cluster limit (opens in a new tab)' });
    expect(link.getAttribute('href')).toBe(
      'https://cost-intelligence-argocd.readthedocs.io/en/stable/how-costs-are-measured/#applications-on-another-cluster',
    );
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });
});
