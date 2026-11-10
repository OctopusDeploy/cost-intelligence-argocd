import React from 'react';
import { useContainerWidth } from '../hooks/useContainerWidth';
import { cardStyle, useTokens } from '../theme/tokens';
import { statColumns } from '../utils/layout';

export interface Stat {
  label: string;
  value: React.ReactNode;
  detail?: React.ReactNode;
}

interface Props {
  stats: Stat[];
  // flush drops the space below the strip, for when it sits in a row that provides its own.
  flush?: boolean;
  // footer is a row across the whole strip, below the stats.
  footer?: { label: string; content: React.ReactNode };
}

// StatStrip is one card of side-by-side figures, styled after Octopus Deploy's summary figures. It folds to
// fit its own width — every stat on one row, then two columns, then one — so it works both across
// a page and inside a narrow column. Dividers sit between stats on the same row only.
export const StatStrip: React.FC<Props> = ({ stats, flush = false, footer }) => {
  const t = useTokens();
  // Until the strip is measured, and wherever there is no layout, assume room for one row.
  const [ref, width] = useContainerWidth<HTMLDListElement>(Number.POSITIVE_INFINITY);
  const columns = statColumns(width, stats.length);

  return (
    <dl
      ref={ref}
      style={{
        ...cardStyle(t),
        display: 'grid',
        gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
        rowGap: '24px',
        margin: flush ? 0 : '0 0 16px',
        padding: footer ? '20px 0 0' : '20px 0',
      }}
    >
      {stats.map((stat, i) => (
        <div
          key={stat.label}
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: '6px',
            minWidth: 0,
            padding: '0 24px',
            borderLeft: i % columns === 0 ? 'none' : `1px solid ${t.hairline}`,
          }}
        >
          <dt
            style={{
              fontSize: '12px',
              fontWeight: 500,
              color: t.textMuted,
            }}
          >
            {stat.label}
          </dt>
          <dd style={{ margin: 0, fontSize: '22px', fontWeight: 600, lineHeight: '28px', color: t.textStrong }}>
            {stat.value}
          </dd>
          {stat.detail && <dd style={{ margin: 0, fontSize: '12px', color: t.textMuted }}>{stat.detail}</dd>}
        </div>
      ))}
      {footer && (
        <div
          style={{
            gridColumn: '1 / -1',
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            gap: '8px 16px',
            padding: '12px 24px',
            borderTop: `1px solid ${t.hairline}`,
            fontSize: '12px',
            color: t.textMuted,
          }}
        >
          <dt style={{ fontWeight: 500 }}>{footer.label}</dt>
          <dd
            style={{
              flex: '1 1 280px',
              display: 'flex',
              flexWrap: 'wrap',
              alignItems: 'center',
              gap: '8px 16px',
              minWidth: 0,
              margin: 0,
            }}
          >
            {footer.content}
          </dd>
        </div>
      )}
    </dl>
  );
};
