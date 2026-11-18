import React, { useMemo } from 'react';
import { CostPanel } from './components/CostPanel';
import { RemoteClusterPanel } from './components/RemoteClusterPanel';
import { AppContext } from './utils/api';
import { Destination, otherCluster } from './utils/destination';
import { useKnownClusters, useMeasuredClusters } from './hooks/useMeasuredClusters';
import { managedControllersOf } from './utils/managedControllers';

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
      resources?: Array<{ kind: string; name: string; namespace?: string }>;
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

  // Derive the set of controller names this ArgoCD app manages so the Resources
  // tab can filter OpenCost pod data down to just this app's resources.
  const managedControllers = useMemo<Set<string>>(
    () => managedControllersOf(application?.status?.resources),
    [application?.status?.resources]
  );

  // Where the application deploys is another cluster unless the service measures that one too.
  const deploysElsewhere = otherCluster(application?.spec?.destination) !== null;
  const measuredClusters = useMeasuredClusters(deploysElsewhere);
  const knownClusters = useKnownClusters(deploysElsewhere);

  if (!appName) {
    return <div>Unable to determine application name</div>;
  }
  if (deploysElsewhere && measuredClusters === null) {
    return null; // the list of measured clusters is still loading
  }

  // OpenCost measures only the cluster the service runs in, so an application deploying elsewhere has
  // no cost to show, and its views would fetch nothing useful. Kubecost measures the clusters of its
  // cluster map as well.
  const cluster = otherCluster(application.spec?.destination, measuredClusters ?? []);
  if (cluster) {
    return <RemoteClusterPanel cluster={cluster} known={knownClusters} mapped={measuredClusters ?? []} />;
  }

  return <CostPanel appName={appName} appCtx={appCtx} managedControllers={managedControllers} />;
};

// Export default for Argo CD to detect
export default {
  component: CostIntelligenceTab,
  name: 'Cost Intelligence',
  key: 'cost-intelligence',
};
