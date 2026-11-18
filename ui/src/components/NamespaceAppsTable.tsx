import React, { useState } from 'react';
import { CardHeader } from './CardHeader';
import { Share } from './ControllerCostTable';
import { AppsIcon, ChevronIcon, WarningIcon } from './icons';
import { cardStyle, useTokens } from '../theme/tokens';
import { formatMoney, formatRelativeTime, isStale, pluralize, roundColumns, roundParts } from '../utils/format';
import { NAME_COLUMN, SPACER_COLUMN } from '../utils/layout';
import { AppCost } from '../utils/namespaceApps';
import { ControllerGroup } from '../utils/podGroups';
import { PodCost } from '../utils/api';

// Expand, application, CPU, memory, (GPU, storage, network, shared,) total, share, measured, then a
// spacer. The application name grows to 480px and the spacer takes any width beyond that (see
// NAME_COLUMN). An optional column is there only when some application has that cost, so the cells
// beside a total add up to it.
const columns = (optional: number) => `48px ${NAME_COLUMN} 104px 104px ${'104px '.repeat(optional)}116px 132px 140px ${SPACER_COLUMN}`;
const minTableWidth = (optional: number) => `${844 + 104 * optional}px`;
const TITLE_ID = 'cost-intel-namespace-apps-title';

interface OptionalColumn {
  label: string;
  title: string;
  app: (app: AppCost) => number;
  controller: (controller: ControllerGroup) => number;
  pod: (pod: PodCost) => number;
}

const OPTIONAL_COLUMNS: OptionalColumn[] = [
  { label: 'GPU', title: 'GPU cost', app: a => a.gpuCost, controller: c => c.gpuCost, pod: p => p.gpuCost ?? 0 },
  { label: 'Storage', title: 'Persistent volume cost', app: a => a.storageCost, controller: c => c.storageCost, pod: p => p.storageCost ?? 0 },
  { label: 'Network', title: 'Network and load balancer cost', app: a => a.networkCost, controller: c => c.networkCost, pod: p => p.networkCost ?? 0 },
  {
    label: 'Shared',
    title: "Cost shared across the cluster, not the app's own usage",
    app: a => a.sharedCost,
    controller: c => c.sharedCost,
    pod: p => p.sharedCost ?? 0,
  },
  {
    label: 'External',
    title: 'External cost: cloud services attributed to the app',
    app: a => a.externalCost,
    controller: c => c.externalCost,
    pod: p => p.externalCost ?? 0,
  },
];

// appLink opens an application's Cost Intelligence view in Argo CD.
const appLink = (app: AppCost) =>
  app.visible && app.name && app.namespace
    ? `/applications/${encodeURIComponent(app.namespace)}/${encodeURIComponent(app.name)}?view=Cost+Intelligence`
    : null;

interface Props {
  apps: AppCost[];
  totalCost: number;
  // unattributed is the part of the namespace's cost that no pod row accounts for, such as volumes
  // no pod mounts. Shown as its own row when it is worth a cent or more.
  unattributed?: number;
}

// NamespaceAppsTable lists a namespace's applications, each expandable to the controllers it manages.
// Every pod appears under exactly one row.
export const NamespaceAppsTable: React.FC<Props> = ({ apps, totalCost, unattributed = 0 }) => {
  const t = useTokens();
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const shown = OPTIONAL_COLUMNS.filter(column => apps.some(app => column.app(app) > 0));

  const toggle = (key: string) =>
    setExpanded(current => {
      const next = new Set(current);
      if (!next.delete(key)) next.add(key);
      return next;
    });

  const row: React.CSSProperties = {
    display: 'grid',
    gridTemplateColumns: columns(shown.length),
    alignItems: 'center',
    color: t.text,
    fontSize: '13px',
    fontVariantNumeric: 'tabular-nums',
  };
  const cell: React.CSSProperties = { minWidth: 0, padding: '0 15px' };
  const numeric: React.CSSProperties = { ...cell, textAlign: 'right' };
  const truncated: React.CSSProperties = { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' };
  const money = (app: AppCost, value: number) => (app.visible ? formatMoney(value) : '—');

  return (
    <section aria-labelledby={TITLE_ID}>
      <CardHeader
        id={TITLE_ID}
        icon={<AppsIcon />}
        title="Applications"
        detail="Each pod counts once, under the app that manages it · totals include storage and network"
      />
      <div style={{ overflowX: 'auto' }}>
        <div role="table" aria-labelledby={TITLE_ID} style={{ minWidth: minTableWidth(shown.length) }}>
          <div role="rowgroup">
            <div
              role="row"
              style={{
                ...row,
                height: '18px',
                margin: '12px 0 8px',
                color: t.textStrong,
                fontSize: '12.8px',
                fontWeight: 600,
                whiteSpace: 'nowrap',
              }}
            >
              <span role="columnheader" aria-label="Controllers" />
              <span role="columnheader" style={cell}>
                App
              </span>
              <span role="columnheader" style={numeric}>
                CPU
              </span>
              <span role="columnheader" style={numeric}>
                Memory
              </span>
              {shown.map(column => (
                <span key={column.label} role="columnheader" style={numeric} title={column.title}>
                  {column.label}
                </span>
              ))}
              <span role="columnheader" style={numeric}>
                Total
              </span>
              <span role="columnheader" style={numeric}>
                Share
              </span>
              <span role="columnheader" style={numeric}>
                Measured
              </span>
            </div>
          </div>

          {apps.map((app, index) => {
            // An application is its namespace and name: with apps-in-any-namespace, team-a/web and
            // team-b/web can both deploy here.
            const key = `${app.namespace ?? ''}/${app.name ?? ''}/${app.group ?? ''}`;
            const open = expanded.has(key);
            const groupId = `cost-intel-namespace-app-${index}`;
            const href = appLink(app);
            const label = app.name ?? app.group ?? 'Not managed by an Argo CD app';
            const detail = app.visible
              ? `${app.group ? 'Not managed by an Argo CD app · ' : ''}${pluralize(app.controllers.length, 'controller')} · ${pluralize(app.podCount, 'pod')}`
              : 'Not listed by Argo CD';
            // Rounded so the cells beside each total add up to it, and the controllers to their app.
            const [appTotal] = roundParts([app.totalCost]);
            const [appCpu, appRam, ...appOptional] = roundColumns(
              [app.cpuCost, app.ramCost, ...shown.map(column => column.app(app))],
              app.totalCost,
              appTotal
            );
            const controllerTotals = roundParts(app.controllers.map(controller => controller.totalCost), appTotal);
            return (
              <div key={key} id={groupId} role="rowgroup" style={{ ...cardStyle(t), margin: '8px 0', overflow: 'hidden' }}>
                <div role="row" style={{ ...row, minHeight: '56px' }}>
                  <span role="cell" style={{ paddingLeft: '12px' }}>
                    {app.controllers.length > 0 && (
                      <button
                        type="button"
                        aria-expanded={open}
                        aria-controls={groupId}
                        aria-label={`Controllers of ${label}`}
                        onClick={() => toggle(key)}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          width: '28px',
                          height: '28px',
                          padding: 0,
                          border: 'none',
                          borderRadius: '4px',
                          backgroundColor: t.surfaceSunken,
                          color: t.text,
                          cursor: 'pointer',
                        }}
                      >
                        <ChevronIcon expanded={open} />
                      </button>
                    )}
                  </span>
                  <span role="rowheader" style={{ ...cell, display: 'flex', flexDirection: 'column', gap: '2px' }}>
                    {href ? (
                      <a href={href} title={label} style={{ ...truncated, color: t.accentText, fontWeight: 500, textDecoration: 'none' }}>
                        {label}
                      </a>
                    ) : (
                      <span title={label} style={{ ...truncated, color: t.textStrong, fontWeight: 500 }}>
                        {label}
                      </span>
                    )}
                    <span style={{ color: t.textMuted, fontSize: '12px' }}>{detail}</span>
                  </span>
                  <span role="cell" style={numeric}>
                    {money(app, appCpu)}
                  </span>
                  <span role="cell" style={numeric}>
                    {money(app, appRam)}
                  </span>
                  {shown.map((column, i) => (
                    <span key={column.label} role="cell" style={numeric}>
                      {money(app, appOptional[i])}
                    </span>
                  ))}
                  <span role="cell" style={{ ...numeric, color: t.textStrong, fontWeight: 500 }}>
                    {money(app, appTotal)}
                  </span>
                  <span role="cell" style={cell}>
                    {app.visible && <Share fraction={app.share} t={t} />}
                  </span>
                  <span role="cell" style={numeric}>
                    {app.measuredAt && (
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', color: t.textMuted, fontSize: '12.8px' }}>
                        {isStale(app.measuredAt) && <WarningIcon color={t.warning} size={12} />}
                        {formatRelativeTime(app.measuredAt)}
                      </span>
                    )}
                  </span>
                </div>

                {open &&
                  app.controllers.map((controller, controllerIndex) => {
                    const controllerTotal = controllerTotals[controllerIndex];
                    const controllerKey = `${key}/${controller.key}`;
                    const podTotals = roundParts(controller.pods.map(pod => pod.totalCost), controllerTotal);
                    const podRows = controller.pods.map((pod, podIndex) => ({
                      pod,
                      total: podTotals[podIndex],
                      values: roundColumns(
                        [pod.cpuCost, pod.ramCost, ...shown.map(column => column.pod(pod))],
                        pod.totalCost,
                        podTotals[podIndex]
                      ),
                    }));
                    const [controllerCpu, controllerRam, ...controllerOptional] = roundColumns(
                      [controller.cpuCost, controller.ramCost, ...shown.map(column => column.controller(controller))],
                      controller.totalCost,
                      controllerTotal
                    );
                    return (
                      <React.Fragment key={controller.key}>
                      <div
                        role="row"
                        style={{ ...row, minHeight: '44px', backgroundColor: t.surfaceSunken, borderTop: `1px solid ${t.border}` }}
                      >
                        <span role="cell" style={{ paddingLeft: '20px' }}>
                          {controller.pods.length > 0 && (
                            <button
                              type="button"
                              aria-expanded={expanded.has(controllerKey)}
                              aria-label={`Pods of ${controller.name}`}
                              onClick={() => toggle(controllerKey)}
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                width: '24px',
                                height: '24px',
                                padding: 0,
                                border: 'none',
                                borderRadius: '4px',
                                backgroundColor: 'transparent',
                                color: t.text,
                                cursor: 'pointer',
                              }}
                            >
                              <ChevronIcon expanded={expanded.has(controllerKey)} />
                            </button>
                          )}
                        </span>
                        <span role="rowheader" style={{ ...cell, ...truncated, paddingLeft: '31px' }}>
                          {controller.name}{' '}
                          <span style={{ color: t.textMuted, fontSize: '12px' }}>
                            {controller.kind} · {pluralize(controller.pods.length, 'pod')}
                          </span>
                        </span>
                        <span role="cell" style={numeric}>
                          {formatMoney(controllerCpu)}
                        </span>
                        <span role="cell" style={numeric}>
                          {formatMoney(controllerRam)}
                        </span>
                        {shown.map((column, i) => (
                          <span key={column.label} role="cell" style={numeric}>
                            {formatMoney(controllerOptional[i])}
                          </span>
                        ))}
                        <span role="cell" style={numeric}>
                          {formatMoney(controllerTotal)}
                        </span>
                        <span role="cell" style={cell}>
                          <Share fraction={totalCost > 0 ? controller.totalCost / totalCost : 0} t={t} />
                        </span>
                        <span role="cell" />
                      </div>
                      {expanded.has(controllerKey) &&
                        podRows.map(({ pod, values, total }) => (
                          <div
                            key={pod.name}
                            role="row"
                            style={{ ...row, minHeight: '36px', borderTop: `1px solid ${t.border}`, color: t.textMuted }}
                          >
                            <span role="cell" />
                            <span role="rowheader" title={pod.name} style={{ ...cell, ...truncated, paddingLeft: '51px' }}>
                              {pod.name}
                            </span>
                            <span role="cell" style={numeric}>
                              {formatMoney(values[0])}
                            </span>
                            <span role="cell" style={numeric}>
                              {formatMoney(values[1])}
                            </span>
                            {shown.map((column, i) => (
                              <span key={column.label} role="cell" style={numeric}>
                                {formatMoney(values[i + 2])}
                              </span>
                            ))}
                            <span role="cell" style={numeric}>
                              {formatMoney(total)}
                            </span>
                            <span role="cell" style={cell}>
                              <Share fraction={totalCost > 0 ? pod.totalCost / totalCost : 0} t={t} />
                            </span>
                            <span role="cell" />
                          </div>
                        ))}
                      </React.Fragment>
                    );
                  })}
              </div>
            );
          })}

          {unattributed >= 0.01 && (
            <div role="rowgroup" style={{ ...cardStyle(t), margin: '8px 0', overflow: 'hidden' }}>
              <div role="row" style={{ ...row, minHeight: '56px' }}>
                <span role="cell" />
                <span role="rowheader" style={{ ...cell, display: 'flex', flexDirection: 'column', gap: '2px' }}>
                  <span style={{ ...truncated, color: t.textStrong, fontWeight: 500 }}>Not attributed to a pod</span>
                  <span style={{ color: t.textMuted, fontSize: '12px' }}>
                    In the namespace's cost, in no pod above, such as volumes no pod mounts
                  </span>
                </span>
                <span role="cell" style={numeric}>
                  —
                </span>
                <span role="cell" style={numeric}>
                  —
                </span>
                {shown.map(column => (
                  <span key={column.label} role="cell" style={numeric}>
                    —
                  </span>
                ))}
                <span role="cell" style={{ ...numeric, color: t.textStrong, fontWeight: 500 }}>
                  {formatMoney(unattributed)}
                </span>
                <span role="cell" style={cell}>
                  <Share fraction={unattributed / (totalCost + unattributed)} t={t} />
                </span>
                <span role="cell" />
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
};
