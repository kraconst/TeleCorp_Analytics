import { apiRequest, withCompany } from './client';
import type { Account } from '@/types/api';

export interface AccountInput {
  employee_id: number;
  phone: string;
  username?: string;
}

export const listAccounts = (companyId: number, signal?: AbortSignal) =>
  apiRequest<Account[]>(withCompany('/accounts', companyId), { signal }).then((data) => data ?? []);

export const getAccount = (companyId: number, accountId: number, signal?: AbortSignal) =>
  apiRequest<Account>(withCompany(`/accounts/${accountId}`, companyId), { signal });

export const createAccount = (companyId: number, input: AccountInput, signal?: AbortSignal) =>
  apiRequest<Account>(withCompany('/accounts', companyId), {
    method: 'POST',
    body: { company_id: companyId, ...input },
    signal,
  });

export const assignAccount = (
  companyId: number,
  accountId: number,
  employeeId: number,
  signal?: AbortSignal,
) =>
  apiRequest<{ status: string }>(withCompany(`/accounts/${accountId}/assign`, companyId), {
    method: 'POST',
    body: { employee_id: employeeId },
    signal,
  });
