import { useEffect, useState } from 'react';
import api from '../utils/api';
import { MeasuredCluster } from '../utils/destination';

// useMeasuredClusters lists the other clusters whose applications the service measures. It asks only
// when needed (an application that deploys elsewhere) and is null until the answer is in.
export function useMeasuredClusters(needed: boolean): MeasuredCluster[] | null {
  const [clusters, setClusters] = useState<MeasuredCluster[] | null>(null);
  useEffect(() => {
    if (!needed) return undefined;
    let current = true;
    void api.getMeasuredClusters().then(list => {
      if (current) setClusters(list);
    });
    return () => {
      current = false;
    };
  }, [needed]);
  return clusters;
}

// useKnownClusters lists the cluster names the cost backend has cost for, to say what a map entry can
// name. It asks only when needed and is empty until the answer is in.
export function useKnownClusters(needed: boolean): string[] {
  const [names, setNames] = useState<string[]>([]);
  useEffect(() => {
    if (!needed) return undefined;
    let current = true;
    void api.getKnownClusters().then(list => {
      if (current) setNames(list);
    });
    return () => {
      current = false;
    };
  }, [needed]);
  return names;
}
