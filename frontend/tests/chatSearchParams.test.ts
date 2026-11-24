import { describe, expect, it } from 'vitest';
import { parseChatUrl, updateSearchParams } from '@/lib/chatSearchParams';

describe('chat URL state', () => {
  it('restores a deep link with filters and selected chat', () => {
    const parsed = parseChatUrl(new URLSearchParams('view=split&status=action_required&employee_id=2&account_id=3&q=Иван&unanswered=true&chat_id=42'));
    expect(parsed).toEqual({ view: 'split', status: 'action_required', employeeId: 2, accountId: 3, query: 'Иван', unanswered: true, chatId: 42 });
  });

  it('drops invalid values instead of redirecting forever', () => {
    expect(parseChatUrl(new URLSearchParams('status=wrong&chat_id=-1&employee_id=nope'))).toMatchObject({ status: undefined, chatId: undefined, employeeId: undefined, view: 'split' });
  });

  it('preserves unrelated filters while changing selection', () => {
    const next = updateSearchParams(new URLSearchParams('status=new_lead&q=Анна&chat_id=1'), { chat_id: 2, view: 'kanban' });
    expect(next.get('status')).toBe('new_lead');
    expect(next.get('q')).toBe('Анна');
    expect(next.get('chat_id')).toBe('2');
    expect(next.get('view')).toBe('kanban');
  });
});
