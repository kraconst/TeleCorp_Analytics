import { useQuery } from '@tanstack/react-query';
import { ApiError } from './client';
import { queryKeys } from './queryKeys';
import { getAccountRatings, getDashboard } from './analytics';
import { listAccounts } from './accounts';
import { listAlerts } from './alerts';
import { getCompany } from './company';
import { listContacts } from './contacts';
import { listEmployees } from './employees';
import { getChat, getChatAnalytics, listChats } from './chats';
import type { DateRange } from '@/types/api';

function retryQuery(failureCount: number, error: Error) {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500 && error.status !== 408) {
    return false;
  }
  return failureCount < 1;
}

export function useAccounts(companyId: number) {
  return useQuery({
    queryKey: queryKeys.accounts(companyId),
    queryFn: ({ signal }) => listAccounts(companyId, signal),
    enabled: companyId > 0,
    staleTime: 60_000,
    retry: retryQuery,
  });
}

export function useEmployees(companyId: number) {
  return useQuery({
    queryKey: queryKeys.employees(companyId),
    queryFn: ({ signal }) => listEmployees(companyId, signal),
    enabled: companyId > 0,
    staleTime: 60_000,
    retry: retryQuery,
  });
}

export function useContacts(companyId: number) {
  return useQuery({
    queryKey: queryKeys.contacts(companyId),
    queryFn: ({ signal }) => listContacts(companyId, signal),
    enabled: companyId > 0,
    staleTime: 60_000,
    retry: retryQuery,
  });
}

export function useCompany(companyId: number) {
  return useQuery({
    queryKey: queryKeys.company(companyId),
    queryFn: ({ signal }) => getCompany(companyId, signal),
    enabled: companyId > 0,
    staleTime: 300_000,
    retry: retryQuery,
  });
}

export function useChats(companyId: number, status?: Parameters<typeof queryKeys.chats>[1], employeeId?: number, enabled = true) {
  return useQuery({
    queryKey: queryKeys.chats(companyId, status, employeeId),
    queryFn: ({ signal }) => listChats(companyId, { status, employeeId }, signal),
    enabled: enabled && companyId > 0,
    retry: retryQuery,
  });
}

export function useChat(companyId: number, chatId?: number) {
  return useQuery({
    queryKey: queryKeys.chat(companyId, chatId ?? 0),
    queryFn: ({ signal }) => getChat(companyId, chatId ?? 0, signal),
    enabled: companyId > 0 && chatId !== undefined,
    retry: retryQuery,
  });
}

export function useChatAnalytics(companyId: number, chatId?: number) {
  return useQuery({
    queryKey: queryKeys.chatAnalytics(companyId, chatId ?? 0),
    queryFn: ({ signal }) => getChatAnalytics(companyId, chatId ?? 0, signal),
    enabled: companyId > 0 && chatId !== undefined,
    retry: retryQuery,
  });
}

export function useAlerts(companyId: number) {
  return useQuery({
    queryKey: queryKeys.alerts(companyId),
    queryFn: ({ signal }) => listAlerts(companyId, signal),
    enabled: companyId > 0,
    retry: retryQuery,
  });
}

export function useDashboard(companyId: number, range: DateRange, enabled = true) {
  return useQuery({
    queryKey: queryKeys.dashboard(companyId, range.from, range.to),
    queryFn: ({ signal }) => getDashboard(companyId, range, signal),
    enabled: enabled && companyId > 0,
    retry: retryQuery,
  });
}

export function useAccountRatings(companyId: number, range: DateRange, enabled = true) {
  return useQuery({
    queryKey: queryKeys.accountRatings(companyId, range.from, range.to),
    queryFn: ({ signal }) => getAccountRatings(companyId, range, signal),
    enabled: enabled && companyId > 0,
    retry: retryQuery,
  });
}
