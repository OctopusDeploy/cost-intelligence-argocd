import React from 'react';
import { CheckIcon, ErrorIcon, WarningIcon } from './icons';
import { Callout } from './Notice';
import { Tokens, useTokens } from '../theme/tokens';
import { Budget, BUDGET_ANNOTATION, BudgetLevel } from '../utils/budget';
import { formatMoney } from '../utils/format';

type SetBudget = Extract<Budget, { kind: 'set' }>;

const LABELS: Record<BudgetLevel, string> = {
  within: 'Within budget',
  close: 'Getting close',
  nearly: 'Nearly at budget',
  over: 'Over budget',
};

const codeStyle: React.CSSProperties = { fontFamily: 'monospace', color: 'inherit', background: 'none', padding: 0 };

// BudgetGauge is the content of the budget row: how the projected monthly cost compares with the budget.
export const BudgetGauge: React.FC<{ budget: Budget }> = ({ budget }) => {
  const t = useTokens();
  if (budget.kind === 'unreadable') {
    return (
      <span style={{ overflowWrap: 'anywhere' }}>
        {budget.value === '' ? (
          'The budget annotation has no value.'
        ) : (
          <>
            The budget annotation&apos;s value, <code style={codeStyle}>{budget.value}</code>, can&apos;t be read.
          </>
        )}{' '}
        Expected a positive number of dollars per month, such as <code style={codeStyle}>500</code>.
      </span>
    );
  }
  return (
    <>
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', color: t.textStrong, fontWeight: 500, whiteSpace: 'nowrap' }}>
        <LevelIcon level={budget.level} t={t} />
        {LABELS[budget.level]}
      </span>
      <Meter budget={budget} t={t} />
      <span>
        {budget.share} · {formatMoney(budget.cost)} of {formatMoney(budget.amount)}/mo, projected
      </span>
    </>
  );
};

const LevelIcon: React.FC<{ level: BudgetLevel; t: Tokens }> = ({ level, t }) => {
  if (level === 'within') return <CheckIcon color={t.success} size={14} />;
  if (level === 'close') return <WarningIcon color={t.warning} size={14} />;
  return <ErrorIcon color={t.danger} size={14} />;
};

const Meter: React.FC<{ budget: SetBudget; t: Tokens }> = ({ budget, t }) => {
  const filled = Math.min(budget.fraction * 100, 100);
  const fill = budget.level === 'within' ? t.accent : budget.level === 'close' ? t.warning : t.danger;
  return (
    <span
      role="meter"
      aria-label="Share of budget"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(filled)}
      aria-valuetext={`${budget.share} of budget`}
      style={{
        display: 'block',
        flex: '1 1 160px',
        minWidth: '120px',
        height: '6px',
        overflow: 'hidden',
        borderRadius: '3px',
        backgroundColor: t.surfaceSunken,
        boxShadow: `inset 0 0 0 1px ${t.border}`,
      }}
    >
      <span style={{ display: 'block', width: `${filled}%`, height: '100%', backgroundColor: fill }} />
    </span>
  );
};

// OverBudgetNotice is the banner shown once the projected monthly cost passes the budget.
export const OverBudgetNotice: React.FC<{ budget: SetBudget }> = ({ budget }) => {
  const t = useTokens();
  return (
    <Callout
      icon={<ErrorIcon color={t.danger} />}
      title="Over budget"
      note={
        <>
          The budget is the <code style={codeStyle}>{BUDGET_ANNOTATION}</code> annotation on this application.
        </>
      }
    >
      The projected monthly cost of {formatMoney(budget.cost)} is {formatMoney(budget.cost - budget.amount)} above this
      application&apos;s {formatMoney(budget.amount)} budget.
    </Callout>
  );
};
