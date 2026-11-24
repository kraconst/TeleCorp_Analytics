import { isChatStatus } from './statusConfig';
import type { ChatFilters } from '@/types/api';

export type ChatView = 'split' | 'kanban';

export interface ParsedChatUrl extends ChatFilters {
  view: ChatView;
  chatId?: number;
}

function positiveInteger(value: string | null): number | undefined {
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

export function parseChatUrl(params: URLSearchParams): ParsedChatUrl {
  const statusValue = params.get('status');
  return {
    view: params.get('view') === 'kanban' ? 'kanban' : 'split',
    status: isChatStatus(statusValue) ? statusValue : undefined,
    employeeId: positiveInteger(params.get('employee_id')),
    accountId: positiveInteger(params.get('account_id')),
    query: params.get('q')?.trim() || undefined,
    unanswered: params.get('unanswered') === 'true' || undefined,
    chatId: positiveInteger(params.get('chat_id')),
  };
}

export function updateSearchParams(
  current: URLSearchParams,
  updates: Record<string, string | number | boolean | undefined>,
): URLSearchParams {
  const next = new URLSearchParams(current);
  Object.entries(updates).forEach(([key, value]) => {
    if (value === undefined || value === '' || value === false) next.delete(key);
    else next.set(key, String(value));
  });
  return next;
}
