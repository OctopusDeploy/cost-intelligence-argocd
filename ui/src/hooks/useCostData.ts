import { useCallback, useEffect, useState } from 'react';
import api, { AppContext, CostData, CostTrend, DailyCost, MonthCost } from '../utils/api';

export interface UseCostDataResult {
  loading: boolean;
  error: string | null;
  data: CostData | null;
  trend: CostTrend | null;
  dailyCosts: DailyCost[] | null;
  // The calendar month so far and projected, null until known or without history.
  month: MonthCost | null;
  refetch: () => Promise<void>;
}

export const useCostData = (
  appName: string | undefined,
  appCtx: AppContext | undefined,
  autoRefetch: boolean = true,
  refetchInterval: number = 30000
): UseCostDataResult => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<CostData | null>(null);
  const [trend, setTrend] = useState<CostTrend | null>(null);
  const [dailyCosts, setDailyCosts] = useState<DailyCost[] | null>(null);
  const [month, setMonth] = useState<MonthCost | null>(null);

  const fetchData = useCallback(async () => {
    if (!appName) return;

    setLoading(true);
    setError(null);

    try {
      const [costData, trendData, dailyData, monthData] = await Promise.all([
        api.getLatestCost(appName, appCtx),
        api.getCostTrend(appName, 30, appCtx),
        api.getDailyCosts(appName, 30, appCtx),
        // The month is an addition: an older service without the route must not break the rest.
        api.getMonthCost(appName, appCtx).catch(() => null),
      ]);

      setData(costData);
      setTrend(trendData);
      setDailyCosts(dailyData?.dailyCosts ?? null);
      setMonth(monthData?.available ? (monthData.month ?? null) : null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch cost data';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [appName, appCtx]);

  useEffect(() => {
    fetchData();

    if (autoRefetch) {
      const interval = setInterval(fetchData, refetchInterval);
      return () => clearInterval(interval);
    }
  }, [fetchData, autoRefetch, refetchInterval]);

  return {
    loading,
    error,
    data,
    trend,
    dailyCosts,
    month,
    refetch: fetchData,
  };
};
