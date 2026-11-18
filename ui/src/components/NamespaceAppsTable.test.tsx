import React from 'react';
import { fireEvent, render, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { NamespaceAppsTable } from './NamespaceAppsTable';
import { AppCost } from '../utils/namespaceApps';

const app = (namespace: string, controller: string): AppCost => ({
  name: 'web',
  namespace,
  visible: true,
  controllers: [
    {
      key: `Deployment/${controller}`,
      kind: 'Deployment',
      name: controller,
      pods: [],
      cpuCost: 1,
      ramCost: 1,
      storageCost: 0,
      networkCost: 0,
      gpuCost: 0,
      sharedCost: 0,
      externalCost: 0,
      totalCost: 2,
      share: 1,
    },
  ],
  podCount: 0,
  cpuCost: 1,
  ramCost: 1,
  gpuCost: 0,
  storageCost: 0,
  networkCost: 0,
  sharedCost: 0,
  externalCost: 0,
  totalCost: 2,
  share: 0.5,
  measuredAt: null,
});

describe('NamespaceAppsTable', () => {
  it('shows the storage and network columns only when an application has that cost', () => {
    const { queryByRole, rerender } = render(<NamespaceAppsTable apps={[app('team-a', 'web')]} totalCost={2} />);
    expect(queryByRole('columnheader', { name: 'Storage' })).toBeNull();
    expect(queryByRole('columnheader', { name: 'Network' })).toBeNull();

    const costly = { ...app('team-a', 'web'), storageCost: 1, networkCost: 0.5, totalCost: 3.5 };
    rerender(<NamespaceAppsTable apps={[costly]} totalCost={3.5} />);
    expect(queryByRole('columnheader', { name: 'Storage' })).not.toBeNull();
    expect(queryByRole('columnheader', { name: 'Network' })).not.toBeNull();
    expect(queryByRole('columnheader', { name: 'GPU' })).toBeNull();
  });

  it('shows an External column only when an application has external cost', () => {
    const { queryByRole, rerender } = render(<NamespaceAppsTable apps={[app('team-a', 'web')]} totalCost={2} />);
    expect(queryByRole('columnheader', { name: 'External' })).toBeNull();

    rerender(<NamespaceAppsTable apps={[{ ...app('team-a', 'web'), externalCost: 4, totalCost: 6 }]} totalCost={6} />);
    expect(queryByRole('columnheader', { name: 'External' })).not.toBeNull();
  });

  it('expands a controller to its pods, with the cost columns of a group the app lists', () => {
    const base = app('team-a', 'web');
    const pod = { name: 'job-1', namespace: 'team-a', controller: '', controllerKind: '', cpuCost: 1, ramCost: 1, networkCost: 4, totalCost: 6, group: 'dag' };
    const grouped = {
      ...base,
      networkCost: 4,
      totalCost: 6,
      controllers: [{ ...base.controllers[0], kind: 'Group', name: 'dag', pods: [pod], networkCost: 4, totalCost: 6 }],
    };
    const { getByRole, queryByText } = render(<NamespaceAppsTable apps={[grouped]} totalCost={6} />);
    fireEvent.click(getByRole('button', { name: 'Controllers of web' }));
    expect(queryByText('job-1')).toBeNull();
    fireEvent.click(getByRole('button', { name: 'Pods of dag' }));
    expect(queryByText('job-1')).not.toBeNull();
    expect(getByRole('columnheader', { name: 'Network' })).toBeTruthy();
  });

  it('expands one of two same-named applications from different namespaces', () => {
    const { getAllByRole } = render(
      <NamespaceAppsTable apps={[app('team-a', 'web-a'), app('team-b', 'web-b')]} totalCost={4} />
    );

    const toggles = getAllByRole('button', { name: 'Controllers of web' });
    expect(toggles).toHaveLength(2);
    fireEvent.click(toggles[0]);

    expect(toggles.map(toggle => toggle.getAttribute('aria-expanded'))).toEqual(['true', 'false']);
  });

  it('rounds the cells so they add up to the total beside them, and the controllers to their app', () => {
    const controller = (name: string) => ({
      key: `Deployment/${name}`,
      kind: 'Deployment',
      name,
      pods: [],
      cpuCost: 0.0435,
      ramCost: 0.0435,
      storageCost: 0,
      networkCost: 0,
      gpuCost: 0,
      sharedCost: 0,
      externalCost: 0,
      totalCost: 0.087,
      share: 0.5,
    });
    const shop = {
      ...app('shop', 'web'),
      controllers: [controller('web'), controller('api')],
      cpuCost: 0.087,
      ramCost: 0.087,
      totalCost: 0.174,
    };
    const { getAllByRole, getByRole } = render(<NamespaceAppsTable apps={[shop]} totalCost={0.174} />);
    fireEvent.click(getByRole('button', { name: 'Controllers of web' }));

    // Each rounded on its own, CPU and memory would show as $0.09 beside a total of $0.17, and so
    // would each controller.
    const amounts = getAllByRole('row')
      .slice(1)
      .map(row => within(row).queryAllByRole('cell').map(cell => cell.textContent).filter(text => text?.startsWith('$')));
    expect(amounts).toEqual([
      ['$0.09', '$0.08', '$0.17'],
      ['$0.05', '$0.04', '$0.09'],
      ['$0.04', '$0.04', '$0.08'],
    ]);
  });

  it('lists the cost no pod accounts for, and nothing when there is none', () => {
    const { queryByText, rerender } = render(<NamespaceAppsTable apps={[app('team-a', 'web')]} totalCost={2} unattributed={3.5} />);
    expect(queryByText('Not attributed to a pod')).not.toBeNull();

    rerender(<NamespaceAppsTable apps={[app('team-a', 'web')]} totalCost={2} unattributed={0.001} />);
    expect(queryByText('Not attributed to a pod')).toBeNull();
  });
});
