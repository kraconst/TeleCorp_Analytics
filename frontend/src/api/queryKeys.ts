import type { ChatStatus } from '@/types/api';

export const queryKeys = {
  accounts: (companyId: number) => ['accounts', companyId] as const,
  account: (companyId: number, accountId: number) => ['accounts', companyId, accountId] as const,
  employees: (companyId: number) => ['employees', companyId] as const,
  contacts: (companyId: number) => ['contacts', companyId] as const,
  chats: (companyId: number, status?: ChatStatus, employeeId?: number) =>
    ['chats', 'list', companyId, { status, employeeId }] as const,
  chatLists: (companyId: number) => ['chats', 'list', companyId] as const,
  chat: (companyId: number, chatId: number) => ['chats', 'detail', companyId, chatId] as const,
  chatAnalytics: (companyId: number, chatId: number) =>
    ['chats', 'analytics', companyId, chatId] as const,
  messages: (companyId: number, chatId: number, total: number) =>
    ['chats', 'messages', companyId, chatId, total] as const,
  summary: (companyId: number, chatId: number) => ['chats', 'summary', companyId, chatId] as const,
  alerts: (companyId: number) => ['alerts', companyId] as const,
  company: (companyId: number) => ['company', companyId] as const,
  dashboard: (companyId: number, from: string, to: string) =>
    ['analytics', 'dashboard', companyId, from, to] as const,
  accountRatings: (companyId: number, from: string, to: string) =>
    ['analytics', 'accounts', companyId, from, to] as const,
};
