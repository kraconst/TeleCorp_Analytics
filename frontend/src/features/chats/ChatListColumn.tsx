import { Filter, Search, Timer, UsersRound } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { ErrorState, Skeleton } from '@/components/ui/States';
import { useSharedNow } from '@/context/NowContext';
import { contactDisplayName } from '@/lib/chatFilters';
import type { ParsedChatUrl } from '@/lib/chatSearchParams';
import { displayUsername, formatRelativeTime, formatWaitingTime } from '@/lib/formatters';
import { chatStatusConfig, chatStatusOrder, getStatusMeta, priorityConfig } from '@/lib/statusConfig';
import { cn } from '@/lib/utils';
import type { Account, ChatListItem, ChatStatus, Employee } from '@/types/api';

function SearchInput({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const timer = useRef<number | undefined>(undefined);
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  useEffect(() => {
    const input = inputRef.current;
    if (input && document.activeElement !== input && input.value !== value) {
      window.clearTimeout(timer.current);
      input.value = value;
    }
  }, [value]);
  return (
    <label className="relative block">
      <span className="sr-only">Поиск по клиенту</span>
      <Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-stone-400" />
      <Input
        ref={inputRef}
        defaultValue={value}
        className="pl-9 pr-3"
        placeholder="Поиск по клиенту"
        onChange={(event) => {
          const next = event.target.value;
          window.clearTimeout(timer.current);
          timer.current = window.setTimeout(() => onChange(next), 250);
        }}
      />
    </label>
  );
}

export function ChatListColumn({
  chats,
  filters,
  accounts,
  employees,
  role,
  thresholdMinutes,
  isLoading,
  error,
  onRetry,
  onFiltersChange,
  onSelect,
}: {
  chats: ChatListItem[];
  filters: ParsedChatUrl;
  accounts: Account[];
  employees: Employee[];
  role: 'owner' | 'manager';
  thresholdMinutes?: number;
  isLoading: boolean;
  error?: string;
  onRetry: () => void;
  onFiltersChange: (updates: Record<string, string | number | boolean | undefined>) => void;
  onSelect: (chatId: number) => void;
}) {
  const now = useSharedNow();
  const activeEmployees = employees.filter((employee) => employee.is_active || employee.id === filters.employeeId);

  return (
    <section className="flex h-full min-h-0 flex-col bg-white" aria-label="Список чатов">
      <div className="shrink-0 space-y-2.5 border-b border-stone-200 p-3">
        <SearchInput value={filters.query ?? ''} onChange={(value) => onFiltersChange({ q: value || undefined, chat_id: undefined })} />
        <div className="grid grid-cols-2 gap-2">
          <Select
            aria-label="Фильтр по статусу"
            value={filters.status ?? ''}
            onChange={(event) => onFiltersChange({ status: (event.target.value || undefined) as ChatStatus | undefined, chat_id: undefined })}
          >
            <option value="">Все статусы</option>
            {chatStatusOrder.map((status) => <option key={status} value={status}>{chatStatusConfig[status].label}</option>)}
          </Select>
          {role === 'owner' ? (
            <Select
              aria-label="Фильтр по сотруднику"
              value={filters.employeeId ?? ''}
              onChange={(event) => onFiltersChange({ employee_id: event.target.value ? Number(event.target.value) : undefined, chat_id: undefined })}
            >
              <option value="">Все сотрудники</option>
              {activeEmployees.map((employee) => <option key={employee.id} value={employee.id}>{employee.name}</option>)}
            </Select>
          ) : (
            <div className="flex h-9 items-center gap-2 truncate rounded-md bg-stone-100 px-3 text-xs text-stone-500" title="Для manager выбор ограничен его сотрудником">
              <UsersRound className="size-3.5 shrink-0" /> Мои чаты
            </div>
          )}
        </div>
        <div className="flex items-center justify-between gap-2">
          <Select
            aria-label="Фильтр по аккаунту"
            className="h-8 border-0 bg-stone-50 pl-2 text-xs"
            value={filters.accountId ?? ''}
            onChange={(event) => onFiltersChange({ account_id: event.target.value ? Number(event.target.value) : undefined, chat_id: undefined })}
          >
            <option value="">Все аккаунты</option>
            {accounts.map((account) => <option key={account.id} value={account.id}>{displayUsername(account.username)}</option>)}
          </Select>
          <label className="flex shrink-0 cursor-pointer items-center gap-1.5 text-[11px] text-stone-600">
            <input
              type="checkbox"
              className="size-3.5 accent-blue-600"
              checked={filters.unanswered ?? false}
              onChange={(event) => onFiltersChange({ unanswered: event.target.checked || undefined, chat_id: undefined })}
            />
            Просроченные
          </label>
        </div>
        {filters.unanswered && thresholdMinutes === undefined && (
          <p className="text-[11px] leading-4 text-amber-700">Порог компании ещё не загружен; фильтр применится после загрузки.</p>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="space-y-3 p-3">{Array.from({ length: 6 }, (_, index) => <Skeleton key={index} className="h-24" />)}</div>
        ) : error ? (
          <ErrorState className="m-3" description={error} onRetry={onRetry} />
        ) : chats.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center p-6 text-center">
            <Filter className="mb-3 size-5 text-stone-400" />
            <p className="text-sm font-medium text-stone-700">Чаты не найдены</p>
            <p className="mt-1 text-xs leading-5 text-stone-400">Измените условия фильтра или сбросьте его.</p>
          </div>
        ) : (
          <ul className="divide-y divide-stone-100">
            {chats.map((chat) => {
              const selected = filters.chatId === chat.id;
              const isOverdue = chat.awaiting_reply_since && thresholdMinutes !== undefined &&
                now.getTime() - new Date(chat.awaiting_reply_since).getTime() > thresholdMinutes * 60_000;
              return (
                <li key={chat.id}>
                  <button
                    className={cn(
                      'relative w-full cursor-pointer px-3 py-3 text-left transition-colors hover:bg-stone-50',
                      selected && 'bg-blue-50/70 before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-blue-600',
                    )}
                    onClick={() => onSelect(chat.id)}
                    aria-current={selected ? 'true' : undefined}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-stone-900">{contactDisplayName(chat)}</p>
                        <p className="mt-0.5 truncate text-[11px] text-stone-400">
                          {displayUsername(chat.contact?.username)} · {chat.employee?.name || chat.account?.employee_name || 'Сотрудник не найден'}
                        </p>
                      </div>
                      <span className="shrink-0 text-[10px] tabular-nums text-stone-400">{formatRelativeTime(chat.last_message_at, now)}</span>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      <Badge className={getStatusMeta(chatStatusConfig, chat.status, 'Неизвестный статус').className}>{getStatusMeta(chatStatusConfig, chat.status, 'Неизвестный статус').label}</Badge>
                      {chat.priority === 'high' && <Badge className={priorityConfig.high.className}>Высокий</Badge>}
                      {chat.unread_count > 0 && <span className="rounded-full bg-blue-600 px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-white">{chat.unread_count}</span>}
                    </div>
                    {chat.awaiting_reply_since && (
                      <p className={cn('mt-2 flex items-center gap-1 text-[11px] tabular-nums text-stone-500', isOverdue && 'font-medium text-red-700')}>
                        <Timer className="size-3" /> Ждёт ответа {formatWaitingTime(chat.awaiting_reply_since, now)}
                      </p>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}
