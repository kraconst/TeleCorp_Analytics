import { useInfiniteQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowUp, File, Image, Info, MessageSquareText, Mic, PanelRightOpen } from 'lucide-react';
import { useLayoutEffect, useRef } from 'react';
import { listMessages } from '@/api/chats';
import { useChat, useChatAnalytics } from '@/api/hooks';
import { queryKeys } from '@/api/queryKeys';
import { MESSAGE_PAGE_SIZE } from '@/config/env';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Select } from '@/components/ui/Select';
import { EmptyState, ErrorState, LoadingState, Skeleton } from '@/components/ui/States';
import { contactDisplayName, mergeMessagesById } from '@/lib/chatFilters';
import { displayUsername, formatDay, formatMessageTime } from '@/lib/formatters';
import { chatStatusConfig, chatStatusOrder, getStatusMeta, statusSourceConfig } from '@/lib/statusConfig';
import { cn } from '@/lib/utils';
import type { ChatListItem, MediaType, Message } from '@/types/api';
import { useUpdateChatStatus } from './useUpdateChatStatus';

function MediaLabel({ type }: { type: MediaType }) {
  const config = {
    photo: { label: 'Фотография', icon: Image },
    document: { label: 'Документ', icon: File },
    voice: { label: 'Голосовое сообщение', icon: Mic },
  }[type] ?? { label: 'Вложение', icon: File };
  const Icon = config.icon;
  return (
    <span className="mb-2 flex items-center gap-2 text-xs font-medium opacity-75">
      <Icon className="size-4" /> {config.label}
    </span>
  );
}

export function MessageStreamColumn({
  companyId,
  chatId,
  item,
  outsideFilter,
  onOpenDetails,
  onBack,
}: {
  companyId: number;
  chatId?: number;
  item?: ChatListItem;
  outsideFilter?: boolean;
  onOpenDetails: () => void;
  onBack: () => void;
}) {
  const detailsQuery = useChat(companyId, chatId);
  const analyticsQuery = useChatAnalytics(companyId, chatId);
  const updateStatus = useUpdateChatStatus(companyId);
  const scrollRef = useRef<HTMLDivElement>(null);
  const prependSnapshot = useRef<{ height: number; top: number } | null>(null);
  const lastChatId = useRef<number | undefined>(undefined);
  const didInitialScroll = useRef(false);
  const total = analyticsQuery.data?.total_messages;
  const messagesQuery = useInfiniteQuery({
    queryKey: queryKeys.messages(companyId, chatId ?? 0, total ?? 0),
    queryFn: ({ pageParam, signal }) => listMessages(companyId, chatId ?? 0, MESSAGE_PAGE_SIZE, pageParam, signal),
    initialPageParam: Math.max((total ?? 0) - MESSAGE_PAGE_SIZE, 0),
    getNextPageParam: (_lastPage, _pages, lastOffset) => lastOffset > 0 ? Math.max(lastOffset - MESSAGE_PAGE_SIZE, 0) : undefined,
    enabled: chatId !== undefined && total !== undefined && !analyticsQuery.isError,
  });
  const details = detailsQuery.data;
  const chat = details ?? item;
  const contact = details?.contact ?? item?.contact ?? null;
  const normalizedItem = item ?? (details ? { ...details, contact, account: null, employee: null } : undefined);
  const messages = mergeMessagesById<Message>(messagesQuery.data?.pages ?? []);

  useLayoutEffect(() => {
    const element = scrollRef.current;
    if (!element || !messagesQuery.data) return;
    if (lastChatId.current !== chatId) {
      lastChatId.current = chatId;
      didInitialScroll.current = false;
      prependSnapshot.current = null;
    }
    if (prependSnapshot.current) {
      const snapshot = prependSnapshot.current;
      element.scrollTop = snapshot.top + element.scrollHeight - snapshot.height;
      prependSnapshot.current = null;
    } else if (!didInitialScroll.current) {
      element.scrollTop = element.scrollHeight;
      didInitialScroll.current = true;
    }
  }, [chatId, messagesQuery.data]);

  if (!chatId) {
    return (
      <section className="hidden h-full min-w-0 flex-1 items-center justify-center bg-stone-50/60 lg:flex">
        <EmptyState icon={MessageSquareText} title="Выберите диалог" description="Откройте чат в списке, чтобы увидеть переписку, саммари и аналитику" />
      </section>
    );
  }

  if (detailsQuery.isLoading && !chat) return <section className="h-full min-w-0 flex-1 bg-white"><LoadingState className="h-full" /></section>;
  if (detailsQuery.isError || !chat) {
    return (
      <section className="flex h-full min-w-0 flex-1 flex-col bg-white p-4">
        <Button variant="ghost" size="sm" className="mb-3 self-start" onClick={onBack}><ArrowLeft className="size-4" /> К списку</Button>
        <ErrorState title="Чат недоступен" description={detailsQuery.error?.message ?? 'Чат не найден'} onRetry={() => void detailsQuery.refetch()} />
      </section>
    );
  }

  const name = contact?.name || (normalizedItem ? contactDisplayName(normalizedItem) : `Контакт #${chat.contact_id}`);

  return (
    <section className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-white" aria-label={`Переписка с ${name}`}>
      <header className="shrink-0 border-b border-stone-200 px-3 py-3 sm:px-4">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" className="size-8 lg:hidden" onClick={onBack} aria-label="Вернуться к списку"><ArrowLeft className="size-4" /></Button>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h2 className="truncate font-display text-sm font-medium text-stone-950 sm:text-base">{name}</h2>
              {outsideFilter && <Badge className="border-amber-200 bg-amber-50 text-amber-800">Вне фильтра</Badge>}
            </div>
            <p className="mt-0.5 truncate text-xs text-stone-500">
              {displayUsername(contact?.username)}
              {item?.employee?.name || item?.account?.employee_name ? ` · ${item.employee?.name || item.account?.employee_name}` : ''}
            </p>
          </div>
          <Button variant="secondary" size="icon" className="size-8 min-[1440px]:hidden" onClick={onOpenDetails} aria-label="Открыть детали чата"><PanelRightOpen className="size-4" /></Button>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Select
            aria-label="Изменить статус чата"
            className={cn('h-8 max-w-52 border text-xs font-medium', getStatusMeta(chatStatusConfig, chat.status, 'Неизвестный статус').className)}
            value={chat.status}
            disabled={updateStatus.isPending}
            onChange={(event) => updateStatus.mutate({ chatId, status: event.target.value as typeof chat.status })}
          >
            {chatStatusOrder.map((status) => <option key={status} value={status}>{chatStatusConfig[status].label}</option>)}
          </Select>
          <span className="text-xs text-stone-500">{statusSourceConfig[chat.status_source] ?? 'Источник неизвестен'}</span>
        </div>
      </header>
      {analyticsQuery.isError ? (
        <div className="flex min-h-0 flex-1 items-center justify-center p-4">
          <ErrorState
            title="Не удалось определить границы истории"
            description={`${analyticsQuery.error.message} Для корректной загрузки последних сообщений нужен total_messages.`}
            onRetry={() => void analyticsQuery.refetch()}
          />
        </div>
      ) : messagesQuery.isLoading || analyticsQuery.isLoading ? (
        <div className="flex-1 space-y-4 p-5"><Skeleton className="ml-auto h-16 w-2/3" /><Skeleton className="h-20 w-3/4" /><Skeleton className="ml-auto h-14 w-1/2" /></div>
      ) : messagesQuery.isError ? (
        <div className="flex min-h-0 flex-1 items-center justify-center p-4"><ErrorState description={messagesQuery.error.message} onRetry={() => void messagesQuery.refetch()} /></div>
      ) : (
        <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto bg-stone-50/50 px-3 py-4 sm:px-5">
          {messagesQuery.hasNextPage && (
            <div className="mb-4 text-center">
              <Button
                variant="secondary"
                size="sm"
                disabled={messagesQuery.isFetchingNextPage}
                onClick={() => {
                  const element = scrollRef.current;
                  if (element) prependSnapshot.current = { height: element.scrollHeight, top: element.scrollTop };
                  void messagesQuery.fetchNextPage();
                }}
              >
                <ArrowUp className="size-4" /> {messagesQuery.isFetchingNextPage ? 'Загружаем…' : 'Более ранние сообщения'}
              </Button>
            </div>
          )}
          {messages.length === 0 ? (
            <EmptyState title="История пока пуста" description="Сообщения появятся здесь после синхронизации с Telegram" />
          ) : (
            <div className="mx-auto max-w-3xl space-y-2">
              {messages.map((message, index) => {
                const previous = messages[index - 1];
                const newDay = !previous || new Date(previous.sent_at).toDateString() !== new Date(message.sent_at).toDateString();
                const outgoing = message.direction === 'out';
                return (
                  <div key={message.id}>
                    {newDay && <div className="my-4 text-center text-xs font-medium text-stone-500">{formatDay(message.sent_at)}</div>}
                    <div className={cn('flex', outgoing ? 'justify-end' : 'justify-start')}>
                      <article className={cn(
                        'max-w-[88%] rounded-lg border px-3 py-2 text-sm shadow-[0_1px_1px_rgba(0,0,0,0.025)] sm:max-w-[78%]',
                        outgoing ? 'rounded-br-sm border-blue-200 bg-blue-50 text-blue-950' : 'rounded-bl-sm border-stone-200 bg-white text-stone-800',
                      )}>
                        {message.media_type && <MediaLabel type={message.media_type} />}
                        {message.text ? <p className="whitespace-pre-wrap break-words leading-5 [overflow-wrap:anywhere]">{message.text}</p> : !message.media_type && <p className="text-stone-500">Сообщение без текста</p>}
                        <time className={cn('mt-1 block text-right text-xs tabular-nums', outgoing ? 'text-blue-700' : 'text-stone-500')} dateTime={message.sent_at}>{formatMessageTime(message.sent_at)}</time>
                      </article>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}
      <footer className="flex shrink-0 items-center justify-center gap-2 border-t border-stone-200 bg-white px-3 py-3 text-center text-xs text-stone-500">
        <Info className="size-4 shrink-0" /> Режим просмотра · сообщения синхронизируются из Telegram
      </footer>
    </section>
  );
}
