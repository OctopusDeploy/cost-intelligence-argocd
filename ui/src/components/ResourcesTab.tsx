import React from 'react';
import { ControllerCostTable } from './ControllerCostTable';
import { ErrorNotice, Notice } from './Notice';
import { OrphanedPodsNotice } from './OrphanedPodsNotice';
import { RightsizingSection } from './RightsizingSection';
import { AppContext, CostData, Rightsizing } from '../utils/api';
import { ManagedResource, ownedByAny } from '../utils/ownership';
import { groupPodsByController } from '../utils/podGroups';

interface ResourcesTabProps {
  data: CostData | null;
  loading: boolean;
  error: string | null;
  managedResources?: ManagedResource[];
  appName?: string;
  appCtx?: AppContext;
  rightsizing?: Rightsizing | null;
}

export const ResourcesTab: React.FC<ResourcesTabProps> = ({
  data,
  loading,
  error,
  managedResources,
  appName,
  appCtx,
  rightsizing = null,
}) => {
  if (loading && !data) return <Notice>Loading resource costs…</Notice>;
  if (error) return <ErrorNotice message={error} />;

  const allPods = data?.pods ?? [];
  if (allPods.length === 0) {
    return <Notice>No resource costs yet. Sync the application in Argo CD to collect pod-level costs.</Notice>;
  }

  // The backend lists the application's own pods. Records stored before it did list the whole
  // namespace: when Argo CD lists the application's controllers, keep only the pods they own, by the
  // rule the backend measures the application with; managedResources is empty until the app has any.
  const pods =
    data?.scope !== 'application' && managedResources && managedResources.length > 0
      ? allPods.filter(pod => ownedByAny(managedResources, pod))
      : allPods;
  const groups = groupPodsByController(pods);
  const namespace = allPods[0].namespace;

  return (
    <div>
      {groups.length > 0 ? (
        <ControllerCostTable groups={groups} />
      ) : (
        <Notice>
          No costs found for this application&apos;s controllers. Check that its pods are running and that it has synced
          recently.
        </Notice>
      )}
      <RightsizingSection rightsizing={rightsizing} />
      {namespace && <OrphanedPodsNotice namespace={namespace} appName={appName} appCtx={appCtx} />}
    </div>
  );
};
