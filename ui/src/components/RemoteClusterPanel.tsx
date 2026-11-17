import React from 'react';
import { CardHeader } from './CardHeader';
import { ClusterIcon } from './icons';
import { framePanelStyle, PluginFrame } from './PluginFrame';
import { LOCAL_NAME, MeasuredCluster } from '../utils/destination';
import { cardStyle, useTokens, visuallyHidden } from '../theme/tokens';

// LIMIT_URL is the user docs for released versions. Bundles already installed keep this URL, so the page
// and its heading must not move without a redirect.
const LIMIT_URL =
  'https://cost-intelligence-argocd.readthedocs.io/en/stable/how-costs-are-measured/#applications-on-another-cluster';

interface Props {
  // cluster is the other cluster the application deploys to, by its name or server.
  cluster: string;
  // known are the cluster names the cost backend has cost for (Kubecost), what a map entry's id can be.
  known?: string[];
  // mapped are the clusters the service's cluster map lists, to tell a map it did not load from a name it did not match.
  mapped?: MeasuredCluster[];
}

// RemoteClusterPanel stands in for the cost views of an application that deploys to another cluster.
// OpenCost measures only the cluster the service runs in, so the service never measures such an
// application and any cost stored for it priced a namespace of the same name here.
export const RemoteClusterPanel: React.FC<Props> = ({ cluster, known = [], mapped = [] }) => {
  const t = useTokens();
  const term: React.CSSProperties = { color: t.textMuted };
  const value: React.CSSProperties = { margin: 0, color: t.textStrong, overflowWrap: 'anywhere' };

  return (
    <PluginFrame status={<span style={{ color: t.textMuted, fontSize: '12.8px' }}>Not measured</span>}>
      <div style={framePanelStyle}>
        <section aria-labelledby="cost-intel-remote-title" style={{ ...cardStyle(t), padding: '20px' }}>
          <CardHeader id="cost-intel-remote-title" title="Deploys to another cluster" icon={<ClusterIcon />} />
          <dl
            style={{
              display: 'grid',
              gridTemplateColumns: 'max-content minmax(0, 1fr)',
              gap: '6px 16px',
              margin: '16px 0',
              fontSize: '13px',
            }}
          >
            <dt style={term}>Destination</dt>
            <dd style={value}>
              <span
                style={{
                  display: 'inline-block',
                  maxWidth: '100%',
                  padding: '1px 9px',
                  border: `1px solid ${t.border}`,
                  borderRadius: '999px',
                  backgroundColor: t.chrome,
                }}
              >
                {cluster}
              </span>
            </dd>
            <dt style={term}>Measured here</dt>
            <dd style={value}>{LOCAL_NAME}</dd>
          </dl>
          <p style={{ margin: '0 0 8px', color: t.text, lineHeight: 1.5 }}>
            Cost Intelligence measures only the cluster it runs in, so this application has no cost data here. A
            namespace with the same name in this cluster is not this application&apos;s.
          </p>
          <p style={{ margin: 0, color: t.textMuted, lineHeight: 1.5 }}>
            To see its costs, install Cost Intelligence and OpenCost in that cluster, or, with Kubecost, add it to
            the cluster map (Helm value openCost.kubecostClusters).{' '}
            <a href={LIMIT_URL} target="_blank" rel="noopener noreferrer" style={{ color: t.accentText }}>
              Single-cluster limit<span style={visuallyHidden}> (opens in a new tab)</span>
            </a>
          </p>
          <p style={{ margin: '8px 0 0', color: t.textMuted, lineHeight: 1.5 }}>
            {mapped.length > 0
              ? `The service's cluster map lists: ${mapped.map(m => m.name || m.server).join(', ')}. None matches ${cluster}.`
              : 'The service reported no cluster map (or its cluster list could not be read), so only this cluster is measured.'}
          </p>
          {known.length > 0 && (
            <p style={{ margin: '8px 0 0', color: t.textMuted, lineHeight: 1.5 }}>
              Kubecost has cost for: {known.join(', ')}. If one of them is this cluster, add an entry with that id
              and the name {cluster} to openCost.kubecostClusters.
            </p>
          )}
        </section>
      </div>
    </PluginFrame>
  );
};
