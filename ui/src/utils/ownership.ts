import { PodCost } from './api';

// ManagedResource is a resource Argo CD lists for an application.
export interface ManagedResource {
  kind: string;
  name: string;
  namespace?: string;
}

// The suffixes of the controllers OpenCost reports for pods created on behalf of a CronJob (its Jobs,
// named <cronjob>-<scheduled time in minutes>) and of an Argo Rollout (its ReplicaSets, named
// <rollout>-<pod template hash>), which Argo CD lists under the parent only. The hash uses
// Kubernetes' safe alphabet, which has no vowels, so a Rollout named web does not claim a ReplicaSet
// named web-api.
const CRON_JOB_RUN = /^-\d+$/;
const ROLLOUT_REPLICA_SET = /^-[bcdfghjklmnpqrstvwxz2456789]{5,10}$/;

const childOf = (controller: string, parent: string, suffix: RegExp) =>
  controller.startsWith(parent) && suffix.test(controller.slice(parent.length));

// ownsPod reports whether a resource Argo CD lists owns a pod OpenCost reports. It is the backend's
// rule (ManagedResources.OwnsPod), so the namespace page counts the same pods as the application's own
// cost records. OpenCost reports a Deployment's pods under the Deployment and a pod without a controller under
// no controller at all. A resource listed without a namespace matches in any namespace.
export function ownsPod(resource: ManagedResource, pod: PodCost): boolean {
  if (resource.namespace && resource.namespace !== pod.namespace) return false;
  const kind = (pod.controllerKind || '').toLowerCase();
  const controller = pod.controller || '';
  switch (resource.kind.toLowerCase()) {
    case 'pod':
      return controller === '' && pod.name === resource.name;
    case 'cronjob':
      return (
        (kind === 'cronjob' && controller === resource.name) ||
        (kind === 'job' && childOf(controller, resource.name, CRON_JOB_RUN))
      );
    case 'rollout':
      return (
        (kind === 'rollout' && controller === resource.name) ||
        (kind === 'replicaset' && childOf(controller, resource.name, ROLLOUT_REPLICA_SET))
      );
    default:
      return kind === resource.kind.toLowerCase() && controller === resource.name;
  }
}

// Kinds whose pods an application's cost is made of, plus bare pods an application manages directly.
const POD_OWNER_KINDS = new Set(['Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'CronJob', 'ReplicaSet', 'Rollout', 'Pod']);

// podOwners keeps the resources that can own pods, dropping Services, ConfigMaps and the like.
export const podOwners = (resources: ManagedResource[]): ManagedResource[] =>
  resources.filter(resource => POD_OWNER_KINDS.has(resource.kind));

// ownedByAny reports whether any of the resources owns the pod.
export const ownedByAny = (resources: ManagedResource[], pod: PodCost): boolean =>
  resources.some(resource => ownsPod(resource, pod));
