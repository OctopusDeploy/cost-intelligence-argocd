import React from 'react';
import { ClusterChoice } from '../utils/api';
import { useTokens } from '../theme/tokens';

interface Props {
  clusters: ClusterChoice[];
  value: string;
  onChange: (id: string) => void;
}

// ClusterPicker chooses which cluster the page shows, when the cost backend measures several.
export const ClusterPicker: React.FC<Props> = ({ clusters, value, onChange }) => {
  const t = useTokens();
  return (
    <>
      <label htmlFor="cost-intel-cluster" style={{ color: t.textMuted, fontSize: '12.8px', marginRight: '6px' }}>
        Cluster
      </label>
      <select
        id="cost-intel-cluster"
        value={value}
        onChange={event => onChange(event.target.value)}
        style={{
          marginRight: '16px',
          padding: '4px 8px',
          border: `1px solid ${t.border}`,
          borderRadius: '6px',
          backgroundColor: t.chrome,
          color: t.textStrong,
          fontFamily: 'inherit',
          fontSize: '12.8px',
        }}
      >
        <option value="">All clusters</option>
        {clusters.map(c => (
          <option key={c.id} value={c.id}>
            {c.local ? `${c.id} (this cluster)` : c.id}
          </option>
        ))}
      </select>
    </>
  );
};
