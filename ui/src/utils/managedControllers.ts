// Kinds that directly own pods: status.resources is filtered to these to find an application's controllers.
const POD_OWNER_KINDS = new Set(['Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'CronJob', 'ReplicaSet', 'Rollout', 'Cluster']);

// managedControllersOf is the set of controller names an Argo CD application manages, so cost data
// recorded for its whole namespace can be narrowed to the application's own pods.
export function managedControllersOf(resources: Array<{ kind: string; name: string }> | undefined): Set<string> {
  return new Set((resources ?? []).filter(r => POD_OWNER_KINDS.has(r.kind)).map(r => r.name));
}
