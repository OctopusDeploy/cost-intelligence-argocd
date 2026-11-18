import React from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ControllerCostTable } from './ControllerCostTable';
import { PodCost } from '../utils/api';

const pod = (name: string): PodCost => ({
  name,
  namespace: 'shop',
  controller: 'web',
  controllerKind: 'Deployment',
  cpuCost: 0.0435,
  ramCost: 0.0435,
  totalCost: 0.087,
});

describe('ControllerCostTable', () => {
  it('rounds the cells so they add up to the total beside them, and the pods to their controller', () => {
    render(
      <ControllerCostTable
        groups={[
          {
            key: 'Deployment/web',
            kind: 'Deployment',
            name: 'web',
            pods: [pod('web-1'), pod('web-2')],
            cpuCost: 0.087,
            ramCost: 0.087,
            storageCost: 0,
            networkCost: 0,
            gpuCost: 0,
            sharedCost: 0,
            externalCost: 0,
            totalCost: 0.174,
            share: 1,
          },
        ]}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: 'Pods of web' }));

    // Each rounded on its own, CPU and memory would show as $0.09 beside a total of $0.17, and so
    // would each pod.
    const amounts = screen
      .getAllByRole('row')
      .slice(1)
      .map(row => within(row).queryAllByRole('cell').map(cell => cell.textContent).filter(text => text?.startsWith('$')));
    expect(amounts).toEqual([
      ['$0.09', '$0.08', '$0.17'],
      ['$0.05', '$0.04', '$0.09'],
      ['$0.04', '$0.04', '$0.08'],
    ]);
  });

  it('adds a Shared column only when shared cost is counted', () => {
    const group = (sharedCost: number) => ({
      key: 'Deployment/web',
      kind: 'Deployment',
      name: 'web',
      pods: [{ ...pod('web-1'), sharedCost }],
      cpuCost: 10,
      ramCost: 5,
      storageCost: 0,
      networkCost: 0,
      gpuCost: 0,
      sharedCost,
      externalCost: 0,
      totalCost: 15 + sharedCost,
      share: 1,
    });

    const { unmount } = render(<ControllerCostTable groups={[group(0)]} />);
    expect(screen.queryByRole('columnheader', { name: 'Shared' })).toBeNull();
    unmount();

    render(<ControllerCostTable groups={[group(7.5)]} />);
    expect(screen.getByRole('columnheader', { name: 'Shared' })).toBeTruthy();
    const cells = within(screen.getAllByRole('row')[1]).getAllByRole('cell').map(cell => cell.textContent);
    expect(cells.filter(text => text?.startsWith('$'))).toEqual(['$10.00', '$5.00', '$7.50', '$22.50']);
  });

  it('adds an External column only when a controller has external cost', () => {
    const group = (externalCost: number) => ({
      key: 'Deployment/web',
      kind: 'Deployment',
      name: 'web',
      pods: [{ ...pod('web-1'), externalCost }],
      cpuCost: 10,
      ramCost: 5,
      storageCost: 0,
      networkCost: 0,
      gpuCost: 0,
      sharedCost: 0,
      externalCost,
      totalCost: 15 + externalCost,
      share: 1,
    });

    const { unmount } = render(<ControllerCostTable groups={[group(0)]} />);
    expect(screen.queryByRole('columnheader', { name: 'External' })).toBeNull();
    unmount();

    render(<ControllerCostTable groups={[group(4)]} />);
    expect(screen.getByRole('columnheader', { name: 'External' })).toBeTruthy();
    const cells = within(screen.getAllByRole('row')[1]).getAllByRole('cell').map(cell => cell.textContent);
    expect(cells.filter(text => text?.startsWith('$'))).toEqual(['$10.00', '$5.00', '$4.00', '$19.00']);
  });

  it('adds Storage and Network columns so the cells add up to the total', () => {
    const data = { ...pod('web-1'), cpuCost: 10, ramCost: 5, storageCost: 4, networkCost: 1, totalCost: 20 };
    render(
      <ControllerCostTable
        groups={[
          {
            key: 'Deployment/web',
            kind: 'Deployment',
            name: 'web',
            pods: [data],
            cpuCost: 10,
            ramCost: 5,
            storageCost: 4,
            networkCost: 1,
            gpuCost: 0,
            sharedCost: 0,
            externalCost: 0,
            totalCost: 20,
            share: 1,
          },
        ]}
      />
    );
    expect(screen.getByRole('columnheader', { name: 'Storage' })).toBeTruthy();
    expect(screen.getByRole('columnheader', { name: 'Network' })).toBeTruthy();
    expect(screen.queryByRole('columnheader', { name: 'GPU' })).toBeNull();
    const cells = within(screen.getAllByRole('row')[1]).getAllByRole('cell').map(cell => cell.textContent);
    expect(cells.filter(text => text?.startsWith('$'))).toEqual(['$10.00', '$5.00', '$4.00', '$1.00', '$20.00']);
  });
});
