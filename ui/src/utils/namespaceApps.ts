import { CostData, PodCost } from './api';
import { ControllerGroup, groupPodsByController } from './podGroups';

// ArgoApplication is an Argo CD application and the resources it manages, as listed by
// Argo CD's /api/v1/applications.
export interface ArgoApplication {
  name: string;
  namespace: string;
  resources: Array<{ kind: string; name: string; namespace?: string }>;
}

export interface AppCost {
  // name is null for the group of pods that no visible application manages.
  name: string | null;
  // group names one of those groups when the pods carry the labels the backend groups them by (a
  // team or project). Unset for an application and for the pods with no group.
  group?: string;
  // namespace is the Application resource's own namespace, used to link to it.
  namespace: string | null;
  // visible is false for an application that has cost records but that Argo CD did not list,
  // for example because the viewer may not see it. Its pods count as unmanaged.
  visible: boolean;
  controllers: ControllerGroup[];
  podCount: number;
  cpuCost: number;
  ramCost: number;
  gpuCost: number;
  storageCost: number;
  networkCost: number;
  sharedCost: number;
  totalCost: number;
  // share is the fraction (0–1) of the namespace's cost.
  share: number;
  measuredAt: string | null;
}

export interface NamespaceBreakdown {
  apps: AppCost[];
  // totalCost sums the namespace's pods, each counted once.
  totalCost: number;
  // latest is the most recent measurement; its pods, efficiency and totals describe the namespace.
  latest: CostData | null;
}

type Resource = ArgoApplication['resources'][number];

// The suffixes of the controllers a provider reports for pods created on behalf of a CronJob (its Jobs,
// <cronjob>-<minutes>) and of a Deployment or Argo Rollout (its ReplicaSets, <name>-<pod template hash>,
// which uses Kubernetes' vowel-free alphabet). The same rules as models.ManagedResources.OwnsPod.
const CRON_JOB_RUN = /^-\d+$/;
const REPLICA_SET_HASH = /^-[bcdfghjklmnpqrstvwxz2456789]{5,10}$/;

const POD_OWNER_KINDS = new Set(['deployment', 'statefulset', 'daemonset', 'replicaset', 'job', 'cronjob', 'rollout', 'pod', 'cluster']);

// The shape of a pod's name after its owner's, for a pod the provider reports without a controller
// (Kubecost's pod allocations carry none). The same rules as models.ManagedResources.OwnerOf.
const REPLICA_POD = /^-[bcdfghjklmnpqrstvwxz2456789]{5,10}-[a-z0-9]{5}$/;
const ORDINAL_POD = /^-\d+$/;
const RANDOM_POD = /^-[a-z0-9]{5}$/;


// ownerOf returns the resource that owns a pod, as the backend decides it when it measures an
// application: kinds compare case-insensitively, a Deployment or Rollout also owns the ReplicaSets
// named after it, a CronJob its Jobs, a bare pod is matched by name, and a pod with no reported
// controller by the shape of its name.
export function ownerOf(resources: Resource[], namespace: string, pod: PodCost): Resource | undefined {
  const kind = (pod.controllerKind ?? '').toLowerCase();
  const controller = pod.controller ?? '';
  const suffixed = (full: string, name: string, suffix: RegExp) => full.startsWith(name) && suffix.test(full.slice(name.length));
  return resources.find(resource => {
    if ((resource.namespace ?? namespace) !== namespace) return false;
    const rk = resource.kind.toLowerCase();
    if (!POD_OWNER_KINDS.has(rk)) return false;
    if (controller === '') {
      if (rk === 'pod') return pod.name === resource.name;
      if (rk === 'deployment' || rk === 'rollout') return suffixed(pod.name, resource.name, REPLICA_POD);
      if (rk === 'statefulset' || rk === 'cluster') return suffixed(pod.name, resource.name, ORDINAL_POD);
      if (rk === 'daemonset' || rk === 'job') return suffixed(pod.name, resource.name, RANDOM_POD);
      return false;
    }
    switch (rk) {
      case 'pod':
        return false;
      case 'cronjob':
        return (kind === 'cronjob' && controller === resource.name) || (kind === 'job' && suffixed(controller, resource.name, CRON_JOB_RUN));
      case 'deployment':
      case 'rollout':
        return (kind === rk && controller === resource.name) || (kind === 'replicaset' && suffixed(controller, resource.name, REPLICA_SET_HASH));
      default:
        return kind === rk && controller === resource.name;
    }
  });
}

// ownedByBackend tells whether the backend listed the pod, or its controller, among those the
// application owns. It knows the labels and kinds the UI cannot.
function ownedByBackend(record: CostData, pod: PodCost): boolean {
  return (record.ownedPods ?? []).some(
    owned =>
      owned.name === pod.name ||
      (Boolean(owned.controller) && owned.controller === pod.controller && (owned.controllerKind ?? '') === (pod.controllerKind ?? ''))
  );
}

// splitNamespaceByApp counts each pod in a namespace once and assigns it to the Argo CD application
// that manages its controller. Every cost record measures the whole namespace, so pods come from the
// most recent record only. Pods whose controller no listed application manages form a final group
// with a null name. A pod claimed by several applications goes to the first by name.
export function splitNamespaceByApp(
  namespace: string,
  records: CostData[],
  argoApps: ArgoApplication[]
): NamespaceBreakdown {
  if (records.length === 0) return { apps: [], totalCost: 0, latest: null };

  const latest = records.reduce((newest, record) =>
    Date.parse(record.measuredAt) > Date.parse(newest.measuredAt) ? record : newest
  );

  const owners = [...records]
    .sort((a, b) => a.applicationName.localeCompare(b.applicationName))
    .map(record => {
      const argoApp = argoApps.find(
        app =>
          app.name === record.applicationName &&
          (!record.applicationNamespace || app.namespace === record.applicationNamespace)
      );
      return { record, argoApp, resources: argoApp?.resources ?? [], pods: [] as PodCost[] };
    });

  const unmanaged: PodCost[] = [];
  for (const pod of latest.pods ?? []) {
    let named = pod;
    const owner = owners.find(candidate => {
      // The backend's own attribution (labels, operator-generated kinds) comes first.
      if (candidate.argoApp && ownedByBackend(candidate.record, pod)) return true;
      const resource = ownerOf(candidate.resources, namespace, pod);
      // A pod reported without a controller is grouped under the one that owns it.
      if (resource && !pod.controller && resource.kind.toLowerCase() !== 'pod') {
        named = { ...pod, controller: resource.name, controllerKind: resource.kind };
      }
      return Boolean(resource);
    });
    (owner ? owner.pods : unmanaged).push(named);
  }

  const toAppCost = (
    name: string | null,
    appNamespace: string | null,
    visible: boolean,
    pods: PodCost[],
    measuredAt: string | null
  ): AppCost => {
    const controllers = groupPodsByController(pods);
    return {
      name,
      namespace: appNamespace,
      visible,
      controllers,
      podCount: controllers.reduce((sum, group) => sum + group.pods.length, 0),
      cpuCost: controllers.reduce((sum, group) => sum + group.cpuCost, 0),
      ramCost: controllers.reduce((sum, group) => sum + group.ramCost, 0),
      gpuCost: controllers.reduce((sum, group) => sum + group.gpuCost, 0),
      storageCost: controllers.reduce((sum, group) => sum + group.storageCost, 0),
      networkCost: controllers.reduce((sum, group) => sum + group.networkCost, 0),
      sharedCost: controllers.reduce((sum, group) => sum + group.sharedCost, 0),
      totalCost: controllers.reduce((sum, group) => sum + group.totalCost, 0),
      share: 0,
      measuredAt,
    };
  };

  const apps = owners
    .map(owner =>
      toAppCost(
        owner.record.applicationName,
        owner.argoApp?.namespace ?? (owner.record.applicationNamespace || null),
        Boolean(owner.argoApp),
        owner.pods,
        owner.record.measuredAt
      )
    )
    .sort((a, b) => b.totalCost - a.totalCost);

  const unmanagedCost = toAppCost(null, null, true, unmanaged.filter(pod => !pod.group), null);
  if (unmanagedCost.podCount > 0) apps.push(unmanagedCost);

  // Pods the backend grouped by label (CI job pods by team and project) follow, largest first.
  const groups = new Map<string, PodCost[]>();
  for (const pod of unmanaged) {
    if (pod.group) groups.set(pod.group, [...(groups.get(pod.group) ?? []), pod]);
  }
  apps.push(
    ...[...groups]
      .map(([group, pods]) => ({ ...toAppCost(null, null, true, pods, null), group }))
      .sort((a, b) => b.totalCost - a.totalCost || (a.group ?? '').localeCompare(b.group ?? ''))
  );

  const totalCost = apps.reduce((sum, app) => sum + app.totalCost, 0);
  for (const app of apps) {
    app.share = totalCost > 0 ? app.totalCost / totalCost : 0;
  }

  return { apps, totalCost, latest };
}
