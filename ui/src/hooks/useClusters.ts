import { useEffect, useState } from 'react';
import api, { ClusterChoice } from '../utils/api';

// useClusters lists the clusters the cluster-wide pages can show, this one first. It is empty when the
// cost backend measures only this cluster, and until the answer is in.
export function useClusters(): ClusterChoice[] {
  const [clusters, setClusters] = useState<ClusterChoice[]>([]);
  useEffect(() => {
    let current = true;
    void api.getClusters().then(list => {
      if (current) setClusters(list);
    });
    return () => {
      current = false;
    };
  }, []);
  return clusters;
}
