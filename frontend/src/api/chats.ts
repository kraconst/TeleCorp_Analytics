import { apiRequest, withCompany } from './client';
import type {
  Chat,
  ChatAnalytics,
  ChatDetails,
  ChatStatus,
  ChatSummary,
  Message,
  RiskLevel,
} from '@/types/api';

export interface ServerChatFilters {
  status?: ChatStatus;
  employeeId?: number;
}

export const listChats = (
  companyId: number,
  filters: ServerChatFilters = {},
  signal?: AbortSignal,
) =>
  apiRequest<Chat[]>(
    withCompany('/chats', companyId, {
      status: filters.status,
      employee_id: filters.employeeId,
    }),
    { signal },
  ).then((data) => data ?? []);

export const getChat = (companyId: number, chatId: number, signal?: AbortSignal) =>
  apiRequest<ChatDetails>(withCompany(`/chats/${chatId}`, companyId), { signal });

export const updateChatStatus = (
  companyId: number,
  chatId: number,
  status: ChatStatus,
  signal?: AbortSignal,
) =>
  apiRequest<{ status: string }>(withCompany(`/chats/${chatId}/status`, companyId), {
    method: 'POST',
    body: { status },
    signal,
  });

export const listMessages = (
  companyId: number,
  chatId: number,
  limit: number,
  offset: number,
  signal?: AbortSignal,
) =>
  apiRequest<Message[]>(withCompany(`/chats/${chatId}/messages`, companyId, { limit, offset }), {
    signal,
  }).then((data) => data ?? []);

export const getChatAnalytics = (companyId: number, chatId: number, signal?: AbortSignal) =>
  apiRequest<ChatAnalytics>(withCompany(`/chats/${chatId}/analytics`, companyId), { signal });

export const getChatSummary = (companyId: number, chatId: number, signal?: AbortSignal) =>
  apiRequest<ChatSummary>(withCompany(`/chats/${chatId}/summary`, companyId), { signal });

export interface SummaryInput {
  topic: string;
  agreements: string;
  next_step: string;
  open_questions: string;
  risk: RiskLevel;
  last_message_id: number;
}

export const upsertChatSummary = (
  companyId: number,
  chatId: number,
  input: SummaryInput,
  signal?: AbortSignal,
) =>
  apiRequest<{ status: string }>(withCompany(`/chats/${chatId}/summary`, companyId), {
    method: 'POST',
    body: input,
    signal,
  });

export const generateChatSummary = (
  companyId: number,
  chatId: number,
  signal?: AbortSignal,
) =>
  apiRequest<ChatSummary>(
    withCompany(`/chats/${chatId}/summary/generate`, companyId, { force: true }),
    { method: 'POST', timeoutMs: 45_000, signal },
  );
