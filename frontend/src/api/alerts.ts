import { apiRequest, withCompany } from './client';
import type { Alert } from '@/types/api';

export const listAlerts = (companyId: number, signal?: AbortSignal) =>
  apiRequest<Alert[]>(withCompany('/alerts', companyId), { signal }).then((data) => data ?? []);

export const resolveAlert = (companyId: number, alertId: number, signal?: AbortSignal) =>
  apiRequest<{ status: string }>(withCompany(`/alerts/${alertId}/resolve`, companyId), {
    method: 'POST',
    signal,
  });
