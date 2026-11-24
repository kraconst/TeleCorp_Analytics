import { Columns3, Download, List, RotateCcw } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { downloadCsv } from '@/lib/csv';
import { displayUsername, formatDateTime } from '@/lib/formatters';
import { chatStatusConfig, getStatusMeta, priorityConfig } from '@/lib/statusConfig';
import { contactDisplayName } from '@/lib/chatFilters';
import type { ChatView } from '@/lib/chatSearchParams';
import type { ChatListItem } from '@/types/api';

export function ChatsToolbar({
  view,
  chats,
  hasFilters,
  onViewChange,
  onReset,
}: {
  view: ChatView;
  chats: ChatListItem[];
  hasFilters: boolean;
  onViewChange: (view: ChatView) => void;
  onReset: () => void;
}) {
  const exportChats = () => {
    downloadCsv(
      'telecorp-chats.csv',
      ['ID чата', 'Клиент', 'Username', 'Менеджер', 'Аккаунт', 'Статус', 'Источник', 'Приоритет', 'Непрочитано', 'Последнее сообщение', 'Ожидает ответа с'],
      chats.map((chat) => [
        chat.id,
        contactDisplayName(chat),
        chat.contact?.username,
        chat.employee?.name ?? chat.account?.employee_name,
        displayUsername(chat.account?.username),
        getStatusMeta(chatStatusConfig, chat.status, 'Неизвестный статус').label,
        chat.status_source,
        getStatusMeta(priorityConfig, chat.priority, 'Неизвестный приоритет').label,
        chat.unread_count,
        formatDateTime(chat.last_message_at),
        formatDateTime(chat.awaiting_reply_since),
      ]),
    );
  };

  return (
    <div className="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-stone-200 bg-white px-3 py-2 sm:px-4">
      <div className="flex items-center gap-2">
        <div className="flex rounded-lg border border-stone-200 bg-stone-50 p-1" aria-label="Представление чатов">
          <Button variant={view === 'split' ? 'secondary' : 'ghost'} size="sm" className="h-7 border-0 px-2.5" onClick={() => onViewChange('split')} aria-pressed={view === 'split'}>
            <List className="size-3.5" /> Список
          </Button>
          <Button variant={view === 'kanban' ? 'secondary' : 'ghost'} size="sm" className="h-7 border-0 px-2.5" onClick={() => onViewChange('kanban')} aria-pressed={view === 'kanban'}>
            <Columns3 className="size-3.5" /> Канбан
          </Button>
        </div>
        <span className="hidden text-xs tabular-nums text-stone-400 sm:inline">{chats.length} чатов</span>
      </div>
      <div className="flex items-center gap-1.5">
        {hasFilters && <Button variant="ghost" size="sm" onClick={onReset}><RotateCcw className="size-3.5" /> Сбросить</Button>}
        <Button variant="secondary" size="sm" onClick={exportChats} disabled={chats.length === 0}>
          <Download className="size-3.5" /> CSV
        </Button>
      </div>
    </div>
  );
}
