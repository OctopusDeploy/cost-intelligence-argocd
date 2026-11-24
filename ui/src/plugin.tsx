import React, { useMemo } from 'react';
import { CostPanel } from './components/CostPanel';
import { RemoteClusterPanel } from './components/RemoteClusterPanel';
import { AppContext } from './utils/api';
import { Destination, otherCluster } from './utils/destination';
import { ManagedResource, podOwners } from './utils/ownership';

// Argo CD Plugin Interface
export const plugin = {
  name: 'cost-intelligence-argocd',
  version: '1.0.0',
  description: 'Display cost metrics for Argo CD applications',
};

// This component renders in the Application details view
// It receives props from Argo CD's plugin system

export interface ApplicationDetailsTabProps {
  application: {
    metadata: {
      name: string;
      namespace: string;
    };
    spec?: {
      project?: string;
      destination?: Destination;
    };
    status?: {
      resources?: ManagedResource[];
    };
  };
}

export const CostIntelligenceTab: React.FC<ApplicationDetailsTabProps> = ({
  application,
}) => {
  const appName = application?.metadata?.name;
  const appNs = application?.metadata?.namespace;
  const appProject = application?.spec?.project;

  // Stable reference — prevents useEffect in useCostData from re-firing on every
  // ArgoCD parent re-render (application status polling, etc.)
  const appCtx = useMemo<AppContext>(
    () => ({
      name: appName ?? '',
      namespace: appNs ?? 'argocd',
      project: appProject ?? 'default',
    }),
    [appName, appNs, appProject]
  );

  // The resources Argo CD lists for the app that can own pods, so the tab can narrow an older record
  // of the whole namespace to the app's own pods.
  const managedResources = useMemo<ManagedResource[]>(
    () => podOwners(application?.status?.resources ?? []),
    [application?.status?.resources]
  );

  if (!appName) {
    return <div>Unable to determine application name</div>;
  }

  // OpenCost measures only the cluster the service runs in, so an application deploying elsewhere has
  // no cost to show, and its views would fetch nothing useful.
  const cluster = otherCluster(application.spec?.destination);
  if (cluster) {
    return <RemoteClusterPanel cluster={cluster} />;
  }

  return <CostPanel appName={appName} appCtx={appCtx} managedResources={managedResources} />;
};

// Export default for Argo CD to detect
export default {
  component: CostIntelligenceTab,
  name: 'Cost Intelligence',
  key: 'cost-intelligence',
};
