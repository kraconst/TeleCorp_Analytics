import { adaptCompany } from './adapters';
import { apiRequest } from './client';
import type { Company, CompanyRaw } from '@/types/api';

export const getCompany = async (companyId: number, signal?: AbortSignal): Promise<Company> => {
  const raw = await apiRequest<CompanyRaw>(`/companies/${companyId}`, { signal });
  return adaptCompany(raw);
};
