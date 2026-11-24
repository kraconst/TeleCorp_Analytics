import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { getChat, listMessages } from '@/api/chats';
import { server } from './server';

describe('chat API contract with raw Go JSON', () => {
  it('reads chat details without transforming snake_case fields', async () => {
    server.use(http.get('*/api/chats/42', ({ request }) => {
      expect(new URL(request.url).searchParams.get('company_id')).toBe('1');
      return HttpResponse.json({ id: 42, account_id: 1, contact_id: 2, status: 'new_lead', status_source: 'auto', priority: 'normal', last_message_at: '2026-11-25T10:00:00Z', awaiting_reply_since: null, unread_count: 0, contact: { id: 2, company_id: 1, telegram_user_id: 7, name: 'Иван', username: 'ivan' } });
    }));
    const chat = await getChat(1, 42);
    expect(chat.contact_id).toBe(2);
    expect(chat.contact?.name).toBe('Иван');
  });

  it('sends the requested ascending-history offset unchanged', async () => {
    server.use(http.get('*/api/chats/42/messages', ({ request }) => {
      const url = new URL(request.url);
      expect(url.searchParams.get('limit')).toBe('50');
      expect(url.searchParams.get('offset')).toBe('70');
      return HttpResponse.json([{ id: 71, chat_id: 42, telegram_message_id: 71, direction: 'in', text: 'Последняя страница', media_type: null, sent_at: '2026-11-25T10:00:00Z' }]);
    }));
    const messages = await listMessages(1, 42, 50, 70);
    expect(messages[0].id).toBe(71);
  });
});
