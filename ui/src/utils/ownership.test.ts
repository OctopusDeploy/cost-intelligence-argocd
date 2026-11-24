import { describe, expect, it } from 'vitest';
import { PodCost } from './api';
import { ManagedResource, ownsPod } from './ownership';

// The same cases as the backend's TestManagedResources_OwnsPod, so the two rules stay in step.
const resources: ManagedResource[] = [
  { kind: 'Deployment', name: 'web', namespace: 'shop' },
  { kind: 'StatefulSet', name: 'db', namespace: 'shop' },
  { kind: 'CronJob', name: 'report', namespace: 'shop' },
  { kind: 'Rollout', name: 'api', namespace: 'shop' },
  { kind: 'Pod', name: 'debug', namespace: 'shop' },
  { kind: 'DaemonSet', name: 'agent' },
];

const pod = (namespace: string, controllerKind: string, controller: string, name: string): PodCost => ({
  name,
  namespace,
  controller,
  controllerKind,
  cpuCost: 0,
  ramCost: 0,
  totalCost: 0,
});

describe('ownsPod', () => {
  it.each([
    ['deployment', pod('shop', 'deployment', 'web', 'web-7d4b8c-xk'), true],
    ['kind compared case-insensitively', pod('shop', 'Deployment', 'web', 'web-7d4b8c-xk'), true],
    ['same name, other kind', pod('shop', 'statefulset', 'web', 'web-0'), false],
    ['same name, other namespace', pod('billing', 'deployment', 'web', 'web-7d4b8c-xk'), false],
    ['statefulset', pod('shop', 'statefulset', 'db', 'db-0'), true],
    ['job of the cronjob', pod('shop', 'job', 'report-29312345', 'report-29312345-abcde'), true],
    ['cronjob reported as itself', pod('shop', 'cronjob', 'report', 'report-29312345-abcde'), true],
    ['job named like the cronjob', pod('shop', 'job', 'report-weekly', 'report-weekly-abcde'), false],
    ['rollout reported as itself', pod('shop', 'rollout', 'api', 'api-6f7c9-x'), true],
    ['replicaset of the rollout', pod('shop', 'replicaset', 'api-6f7c9b8d5', 'api-6f7c9b8d5-x'), true],
    ['replicaset named like the rollout', pod('shop', 'replicaset', 'api-gateway', 'api-gateway-x'), false],
    ['bare pod', pod('shop', '', '', 'debug'), true],
    ['bare pod of another name', pod('shop', '', '', 'debug-2'), false],
    ['pod of that name with a controller', pod('shop', 'job', 'cleanup', 'debug'), false],
    ['resource listed without a namespace', pod('anywhere', 'daemonset', 'agent', 'agent-x'), true],
    ['unmanaged controller', pod('shop', 'deployment', 'argocd-redis', 'argocd-redis-x'), false],
  ])('%s', (_name, candidate, want) => {
    expect(resources.some(resource => ownsPod(resource, candidate))).toBe(want);
  });
});
