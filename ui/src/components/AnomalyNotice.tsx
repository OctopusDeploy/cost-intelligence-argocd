import React from 'react';
import { ErrorIcon, WarningIcon } from './icons';
import { Callout } from './Notice';
import { useTokens } from '../theme/tokens';
import { CostAnomaly } from '../utils/api';
import { formatMoney } from '../utils/format';

// AnomalyNotice reports a cost that departs from the rolling average of recent syncs.
export const AnomalyNotice: React.FC<{ anomaly: CostAnomaly }> = ({ anomaly }) => {
  const t = useTokens();
  const alert = anomaly.severity === 'alert';

  return (
    <Callout
      icon={alert ? <ErrorIcon color={t.danger} /> : <WarningIcon color={t.warning} size={16} />}
      title={alert ? 'Cost alert' : 'Cost warning'}
      note={
        <>
          Rolling average over the last {anomaly.recordCount} syncs: {formatMoney(anomaly.rollingAvg)}/mo
        </>
      }
    >
      {anomaly.message}
    </Callout>
  );
};
