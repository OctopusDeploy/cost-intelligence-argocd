import React, { useMemo } from 'react';
import { useCostData } from '../hooks/useCostData';
import { useMeasuredClusters } from '../hooks/useMeasuredClusters';
import { AppContext } from '../utils/api';
import { asApplication } from '../utils/costInsights';
import { Destination, otherCluster } from '../utils/destination';
import { summarizeCostTrend, TrendLevel } from '../utils/costTrend';
import { formatMoney, formatSignedPercent, sumRounded } from '../utils/format';
import { managedControllersOf } from '../utils/managedControllers';

// The colours of Argo CD's own status icons, and of its item titles (ARGO_GRAY6_COLOR).
const LEVEL_COLORS: Record<TrendLevel, string> = { ok: '#18be94', warning: '#f4c030', critical: '#e96d76' };
const TITLE_COLOR = '#6d7f8b';
// useCostData asks for the last 30 days of daily costs.
const TREND_WINDOW_DAYS = 30;

interface Props {
  application: {
    metadata: { name: string; namespace: string };
    spec?: { project?: string; destination?: Destination };
    status?: { resources?: Array<{ kind: string; name: string; namespace?: string }> };
  };
}

// AppCostStatus is the application's monthly cost, shown in Argo CD's status panel next to the last
// sync. It is the figure the Cost Intelligence tab calls Monthly cost: Kubernetes (which already counts
// the idle part of the CPU and memory requests) plus the cloud estimate. Idle is detail, not an addend.
const AppCostStatusItem: React.FC<Props> = ({ application }) => {
  const name = application?.metadata?.name;
  const namespace = application?.metadata?.namespace;
  const project = application?.spec?.project;
  const appCtx = useMemo<AppContext>(
    () => ({ name: name ?? '', namespace: namespace ?? 'argocd', project: project ?? 'default' }),
    [name, namespace, project]
  );
  const managed = useMemo(() => managedControllersOf(application?.status?.resources), [application?.status?.resources]);

  // An application deploying to a cluster the service does not measure has no cost to show.
  const destination = application?.spec?.destination;
  const deploysElsewhere = otherCluster(destination) !== null;
  const measuredClusters = useMeasuredClusters(deploysElsewhere);
  const remote = deploysElsewhere && (measuredClusters === null || otherCluster(destination, measuredClusters) !== null);

  const { data, dailyCosts, month } = useCostData(remote ? undefined : name, appCtx, true, 60000);
  if (remote || !data) return null;

  const latest = asApplication(data, managed);
  if (!latest) return null;
  const kubernetes = latest.totalMonthlyCost;
  const cloud = data.cloudEstimate?.totalMonthlyCost ?? 0;
  const idle = data.efficiency ? data.efficiency.cpuIdleCost + data.efficiency.ramIdleCost : null;
  const detail = [
    cloud > 0 ? `Kubernetes ${formatMoney(kubernetes)} + cloud ${formatMoney(cloud)}` : 'Kubernetes',
    idle !== null && data.scope === 'application' ? `incl. ${formatMoney(idle)} idle` : null,
  ]
    .filter(Boolean)
    .join(', ');
  // The calendar month, which is what the cloud bill covers: cost so far, and the month at the run rate.
  const monthLine = month
    ? `${formatMoney(month.toDate)} so far this month, ${formatMoney(month.projected)} projected`
    : null;

  // Without enough history the trend is unknown, and the icon stays green.
  const summary = dailyCosts ? summarizeCostTrend(dailyCosts, TREND_WINDOW_DAYS) : null;
  const level = summary?.level ?? 'ok';

  // The same markup and spacing as Argo CD's own items (App Health, Sync Status): a title in a
  // 19.5px line, a value with an icon, and the detail right under it. Argo CD gives `item-name` an
  // auto top margin, which centres it in a taller item and pushes it below its neighbours' detail
  // lines, so only the bottom margin is set.
  return (
    <div className="application-status-panel__item" style={{ flex: '0 1 auto', minWidth: 0 }}>
      <div style={{ lineHeight: '19.5px', marginBottom: '0.3em' }}>
        {/* Argo CD styles its item titles through `label`, which here labels no control. */}
        {/* eslint-disable-next-line jsx-a11y/label-has-associated-control */}
        <label style={{ fontSize: '12px', fontWeight: 600, color: TITLE_COLOR }}>MONTHLY COST</label>
      </div>
      <div className="application-status-panel__item-value">
        <i className="fa fa-coins" aria-hidden="true" style={{ color: LEVEL_COLORS[level] }} />
        &nbsp;<span>{formatMoney(sumRounded([kubernetes, cloud]))}</span>
        {summary && (
          <span
            className="application-status-panel__item-value__revision"
            title="Daily cost over the last 7 days compared with the 7 days before"
            style={{ color: LEVEL_COLORS[level] }}
          >
            {formatSignedPercent(summary.change)}
          </span>
        )}
      </div>
      <div className="application-status-panel__item-name" style={{ margin: '0 0 0.5em' }}>
        {detail}
      </div>
      {monthLine && (
        <div
          className="application-status-panel__item-name"
          title="Cost recorded from the 1st of the month (UTC) through today, and the month at the average daily cost of the last 7 days"
          style={{ margin: '0 0 0.5em' }}
        >
          {monthLine}
        </div>
      )}
    </div>
  );
};

// The item renders inside Argo CD's own status panel. React unmounts the whole page when a component
// throws, so a failure here must stay here: the item disappears and Argo CD carries on.
class StatusBoundary extends React.Component<{ children: React.ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    console.error('Cost Intelligence: the status panel item failed to render', error);
  }

  render() {
    return this.state.failed ? null : this.props.children;
  }
}

export const AppCostStatus: React.FC<Props> = props => (
  <StatusBoundary>
    <AppCostStatusItem {...props} />
  </StatusBoundary>
);
