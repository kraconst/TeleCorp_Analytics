import { apiRequest, withCompany } from './client';
import { adaptAccountRating, adaptDashboard } from './adapters';
import type {
  AccountRating,
  AccountRatingRaw,
  DashboardStats,
  DashboardStatsRaw,
  DateRange,
} from '@/types/api';

export async function getDashboard(
  companyId: number,
  range: DateRange,
  signal?: AbortSignal,
): Promise<DashboardStats> {
  const raw = await apiRequest<DashboardStatsRaw>(
    withCompany('/analytics/dashboard', companyId, { from: range.from, to: range.to }),
    { signal },
  );
  return adaptDashboard(raw);
}

export async function getAccountRatings(
  companyId: number,
  range: DateRange,
  signal?: AbortSignal,
): Promise<AccountRating[]> {
  const raw = await apiRequest<AccountRatingRaw[]>(
    withCompany('/analytics/accounts', companyId, { from: range.from, to: range.to }),
    { signal },
  );
  return (raw ?? []).map(adaptAccountRating);
}
