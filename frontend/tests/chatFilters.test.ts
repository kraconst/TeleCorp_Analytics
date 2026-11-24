import { describe, expect, it } from 'vitest';
import { enrichChats, filterChats, mergeMessagesById } from '@/lib/chatFilters';
import type { Account, Chat, Contact, Employee } from '@/types/api';

const account: Account = { id: 1, company_id: 1, employee_id: 2, phone: '', username: '@sales', employee_name: '', telegram_user_id: 1, status: 'active', last_activity_at: null, created_at: '' };
const employee: Employee = { id: 2, company_id: 1, name: 'Анна', position: '', is_active: true };
const contact: Contact = { id: 3, company_id: 1, telegram_user_id: 4, name: 'Алексей Смирнов', username: 'alex' };
const chat: Chat = { id: 5, account_id: 1, contact_id: 3, status: 'action_required', status_source: 'auto', priority: 'high', last_message_at: '2026-11-25T10:00:00Z', awaiting_reply_since: '2026-11-25T09:00:00Z', unread_count: 1 };

describe('chat view models', () => {
  it('joins contact, account and employee without extra requests', () => {
    const [item] = enrichChats([chat], [contact], [account], [employee]);
    expect(item.contact?.name).toBe('Алексей Смирнов');
    expect(item.employee?.name).toBe('Анна');
  });

  it('filters by client and account', () => {
    const [item] = enrichChats([chat], [contact], [account], [employee]);
    expect(filterChats([item], { query: 'смир', accountId: 1 })).toHaveLength(1);
    expect(filterChats([item], { query: 'другой' })).toHaveLength(0);
    expect(filterChats([item], { accountId: 9 })).toHaveLength(0);
  });

  it('uses a strict overdue threshold and waits for threshold data', () => {
    const [item] = enrichChats([chat], [contact], [account], [employee]);
    const now = new Date('2026-11-25T11:00:00Z');
    expect(filterChats([item], { unanswered: true }, now)).toHaveLength(0);
    expect(filterChats([item], { unanswered: true, noReplyThresholdMinutes: 120 }, now)).toHaveLength(0);
    expect(filterChats([item], { unanswered: true, noReplyThresholdMinutes: 119 }, now)).toHaveLength(1);
  });

  it('reverses infinite pages into chronological history and deduplicates', () => {
    const latest = [{ id: 4 }, { id: 5 }];
    const older = [{ id: 2 }, { id: 3 }, { id: 4 }];
    expect(mergeMessagesById([latest, older]).map((message) => message.id)).toEqual([2, 3, 4, 5]);
  });
});
