import type { Account, Chat, ChatFilters, ChatListItem, Contact, Employee } from '@/types/api';

export function enrichChats(
  chats: Chat[],
  contacts: Contact[],
  accounts: Account[],
  employees: Employee[],
): ChatListItem[] {
  const contactsById = new Map(contacts.map((contact) => [contact.id, contact]));
  const accountsById = new Map(accounts.map((account) => [account.id, account]));
  const employeesById = new Map(employees.map((employee) => [employee.id, employee]));
  return chats.map((chat) => {
    const account = accountsById.get(chat.account_id) ?? null;
    return {
      ...chat,
      contact: contactsById.get(chat.contact_id) ?? null,
      account,
      employee: account ? (employeesById.get(account.employee_id) ?? null) : null,
    };
  });
}

export function filterChats(chats: ChatListItem[], filters: ChatFilters, now = new Date()): ChatListItem[] {
  const query = filters.query?.trim().toLocaleLowerCase('ru-RU');
  return chats.filter((chat) => {
    if (filters.status && chat.status !== filters.status) return false;
    if (filters.employeeId && chat.account?.employee_id !== filters.employeeId) return false;
    if (filters.accountId && chat.account_id !== filters.accountId) return false;
    if (query) {
      const haystack = `${chat.contact?.name ?? ''} ${chat.contact?.username ?? ''}`.toLocaleLowerCase('ru-RU');
      if (!haystack.includes(query)) return false;
    }
    if (filters.unanswered) {
      if (filters.noReplyThresholdMinutes === undefined || !chat.awaiting_reply_since) return false;
      const waitingMs = now.getTime() - new Date(chat.awaiting_reply_since).getTime();
      if (waitingMs <= filters.noReplyThresholdMinutes * 60_000) return false;
    }
    return true;
  });
}

export function contactDisplayName(chat: Pick<ChatListItem, 'contact' | 'contact_id'>): string {
  return chat.contact?.name || `Контакт #${chat.contact_id}`;
}

export function mergeMessagesById<T extends { id: number }>(pages: T[][]): T[] {
  const messages = new Map<number, T>();
  pages
    .slice()
    .reverse()
    .flat()
    .forEach((message) => messages.set(message.id, message));
  return [...messages.values()];
}
