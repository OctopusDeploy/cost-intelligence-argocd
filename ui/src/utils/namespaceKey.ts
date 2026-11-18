import { NamespaceSummary } from './api';

// A namespace is its cluster and its name: two clusters can run namespaces of the same name. The page
// selects one by a key, "<cluster>/<namespace>", which is the bare name when no cluster is recorded
// (a single-cluster backend, or costs stored before it was). A namespace name can't contain "/", so the
// key splits at its last one.
export const namespaceKey = (ns: Pick<NamespaceSummary, 'namespace' | 'cluster'>): string =>
  ns.cluster ? `${ns.cluster}/${ns.namespace}` : ns.namespace;

export function splitNamespaceKey(key: string): { cluster?: string; namespace: string } {
  const slash = key.lastIndexOf('/');
  return slash < 0 ? { namespace: key } : { cluster: key.slice(0, slash), namespace: key.slice(slash + 1) };
}

// findNamespace returns the listed namespace a key names. A bare name, as an application's namespace
// is linked by, names the first namespace of that name (the list is ordered by cost).
export function findNamespace<T extends Pick<NamespaceSummary, 'namespace' | 'cluster'>>(list: T[], key: string): T | undefined {
  return list.find(ns => namespaceKey(ns) === key) ?? list.find(ns => ns.namespace === key);
}
