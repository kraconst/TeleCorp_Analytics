import { DndContext, KeyboardSensor, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { CSS } from '@dnd-kit/utilities';
import { ArrowUpRight, GripVertical, Timer } from 'lucide-react';
import { Badge } from '@/components/ui/Badge';
import { useSharedNow } from '@/context/NowContext';
import { contactDisplayName } from '@/lib/chatFilters';
import { displayUsername, formatRelativeTime, formatWaitingTime } from '@/lib/formatters';
import { chatStatusConfig, chatStatusOrder, getStatusMeta, priorityConfig } from '@/lib/statusConfig';
import { cn } from '@/lib/utils';
import type { ChatListItem, ChatStatus } from '@/types/api';
import { useUpdateChatStatus } from '@/features/chats/useUpdateChatStatus';

function KanbanCard({ chat, onOpen }: { chat: ChatListItem; onOpen: () => void }) {
  const now = useSharedNow();
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `chat-${chat.id}`,
    data: { chatId: chat.id, status: chat.status },
  });
  return (
    <article
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform) }}
      className={cn('rounded-lg border border-stone-200 bg-white p-3 shadow-[0_1px_2px_rgba(0,0,0,0.035)]', isDragging && 'z-20 opacity-70 shadow-lg')}
    >
      <div className="flex items-start gap-2">
        <button className="-ml-1 mt-0.5 cursor-grab touch-none rounded p-1 text-stone-500 hover:bg-stone-100 hover:text-stone-700 active:cursor-grabbing" aria-label={`Перетащить чат с ${contactDisplayName(chat)}`} {...listeners} {...attributes}>
          <GripVertical className="size-4" />
        </button>
        <button className="min-w-0 flex-1 cursor-pointer text-left" onClick={onOpen}>
          <span className="block truncate text-sm font-medium text-stone-900">{contactDisplayName(chat)}</span>
          <span className="mt-0.5 block truncate text-xs text-stone-500">{displayUsername(chat.account?.username)} · {chat.employee?.name || chat.account?.employee_name || 'Без имени'}</span>
        </button>
        <button className="rounded p-1 text-stone-500 hover:bg-stone-100 hover:text-blue-600" onClick={onOpen} aria-label={`Открыть чат с ${contactDisplayName(chat)}`}><ArrowUpRight className="size-4" /></button>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        {chat.priority !== 'normal' && <Badge className={getStatusMeta(priorityConfig, chat.priority, 'Неизвестный приоритет').className}>{getStatusMeta(priorityConfig, chat.priority, 'Неизвестный приоритет').label}</Badge>}
        {chat.unread_count > 0 && <Badge className="border-blue-200 bg-blue-50 text-blue-700">{chat.unread_count} новых</Badge>}
        <span className="ml-auto text-xs tabular-nums text-stone-500">{formatRelativeTime(chat.last_message_at, now)}</span>
      </div>
      {chat.awaiting_reply_since && <p className="mt-2 flex items-center gap-1 text-xs tabular-nums text-amber-700"><Timer className="size-4" /> Ждёт {formatWaitingTime(chat.awaiting_reply_since, now)}</p>}
    </article>
  );
}

function KanbanColumn({ status, chats, onOpen }: { status: ChatStatus; chats: ChatListItem[]; onOpen: (id: number) => void }) {
  const { setNodeRef, isOver } = useDroppable({ id: status });
  return (
    <section className="flex h-full w-[288px] shrink-0 flex-col rounded-2xl border border-stone-200 bg-stone-100/70" aria-label={chatStatusConfig[status].label}>
      <header className="flex h-11 shrink-0 items-center justify-between border-b border-stone-200 px-3">
        <div className="flex items-center gap-2"><span className={cn('size-2 rounded-full border', chatStatusConfig[status].className)} /><h3 className="text-xs font-medium text-stone-700">{chatStatusConfig[status].label}</h3></div>
        <span className="text-xs font-medium tabular-nums text-stone-500">{chats.length}</span>
      </header>
      <div ref={setNodeRef} className={cn('min-h-24 flex-1 space-y-2 overflow-y-auto p-2 transition-colors', isOver && 'bg-blue-50')}>
        {chats.map((chat) => <KanbanCard key={chat.id} chat={chat} onOpen={() => onOpen(chat.id)} />)}
        {chats.length === 0 && <div className="grid h-24 place-items-center rounded-lg border border-dashed border-stone-300 text-xs text-stone-500">Перетащите карточку сюда</div>}
      </div>
    </section>
  );
}

export function KanbanBoard({ companyId, chats, onOpen }: { companyId: number; chats: ChatListItem[]; onOpen: (chatId: number) => void }) {
  const updateStatus = useUpdateChatStatus(companyId);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor),
  );
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || !chatStatusOrder.includes(over.id as ChatStatus)) return;
    const chatId = active.data.current?.chatId as number | undefined;
    const previousStatus = active.data.current?.status as ChatStatus | undefined;
    const status = over.id as ChatStatus;
    if (chatId && previousStatus !== status) updateStatus.mutate({ chatId, status });
  };
  return (
    <DndContext sensors={sensors} onDragEnd={onDragEnd}>
      <div className="flex h-full min-h-0 gap-3 overflow-x-auto overflow-y-hidden p-3" aria-label="Канбан чатов">
        {chatStatusOrder.map((status) => <KanbanColumn key={status} status={status} chats={chats.filter((chat) => chat.status === status)} onOpen={onOpen} />)}
      </div>
    </DndContext>
  );
}
