// An Application's destination, as Argo CD has it in spec.destination.
export interface Destination {
  server?: string;
  name?: string;
  namespace?: string;
}

// The cluster the service runs in, as Argo CD names it.
export const LOCAL_SERVER = 'https://kubernetes.default.svc';
export const LOCAL_NAME = 'in-cluster';

// A cluster other than the one the service runs in whose applications it measures: Kubecost with a
// cluster map. Argo CD names a destination by its server or its registered name; either matches.
export interface MeasuredCluster {
  server?: string;
  name?: string;
}

// otherCluster returns the cluster an application deploys to when it is not the one Cost Intelligence
// runs in, by its name or else its server, or null when it deploys here. The rule is the service's:
// local means the server is empty or LOCAL_SERVER and the name is empty or LOCAL_NAME.
export function otherCluster(destination?: Destination, measured: MeasuredCluster[] = []): string | null {
  const server = destination?.server ?? '';
  const name = destination?.name ?? '';
  if ((server === '' || server === LOCAL_SERVER) && (name === '' || name === LOCAL_NAME)) {
    return null;
  }
  if (measured.some(m => (server !== '' && m.server === server) || (name !== '' && m.name !== undefined && m.name.toLowerCase() === name.toLowerCase()))) {
    return null;
  }
  return name || server;
}
