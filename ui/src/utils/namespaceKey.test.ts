import { describe, expect, it } from 'vitest';
import { findNamespace, namespaceKey, splitNamespaceKey } from './namespaceKey';

describe('namespace keys', () => {
  it('are the bare name without a cluster, and cluster/name with one', () => {
    expect(namespaceKey({ namespace: 'argocd' })).toBe('argocd');
    expect(namespaceKey({ namespace: 'argocd', cluster: 'eks-a' })).toBe('eks-a/argocd');
  });

  it('split at the last slash, so a cluster id may contain one', () => {
    expect(splitNamespaceKey('argocd')).toEqual({ namespace: 'argocd' });
    expect(splitNamespaceKey('arn:aws/eks-a/argocd')).toEqual({ cluster: 'arn:aws/eks-a', namespace: 'argocd' });
  });

  it('find a namespace by key, or by bare name as an application links to it', () => {
    const list = [
      { namespace: 'monitoring', cluster: 'eks-a' },
      { namespace: 'monitoring', cluster: 'eks-b' },
    ];
    expect(findNamespace(list, 'eks-b/monitoring')).toBe(list[1]);
    expect(findNamespace(list, 'monitoring')).toBe(list[0]);
    expect(findNamespace(list, 'eks-c/monitoring')).toBeUndefined();
  });
});
