import { AlertCircle, UserRoundCheck } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useAccounts, useChats, useCompany, useContacts, useEmployees } from '@/api/hooks';
import { Dialog } from '@/components/ui/Dialog';
import { EmptyState } from '@/components/ui/States';
import { useDevSession } from '@/context/DevSessionContext';
import { useSharedNow } from '@/context/NowContext';
import { enrichChats, filterChats } from '@/lib/chatFilters';
import { parseChatUrl, updateSearchParams } from '@/lib/chatSearchParams';
import { KanbanBoard } from '@/features/kanban/KanbanBoard';
import { ChatListColumn } from './ChatListColumn';
import { ChatRightPanel } from './ChatRightPanel';
import { ChatsToolbar } from './ChatsToolbar';
import { MessageStreamColumn } from './MessageStreamColumn';

export function ChatsWorkspacePage() {
  const { session } = useDevSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = useMemo(() => parseChatUrl(searchParams), [searchParams]);
  const now = useSharedNow();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const employeeId = session.role === 'manager' ? (session.employeeId ?? undefined) : filters.employeeId;
  const enabled = session.role === 'owner' || session.employeeId !== null;
  const chatsQuery = useChats(session.companyId, filters.status, employeeId, enabled);
  const allChatsQuery = useChats(session.companyId, undefined, employeeId, enabled);
  const accountsQuery = useAccounts(session.companyId);
  const employeesQuery = useEmployees(session.companyId);
  const contactsQuery = useContacts(session.companyId);
  const companyQuery = useCompany(session.companyId);

  const updateParams = useCallback((updates: Record<string, string | number | boolean | undefined>, replace = false) => {
    setSearchParams((current) => updateSearchParams(current, updates), { replace });
  }, [setSearchParams]);

  useEffect(() => {
    if (session.role === 'manager' && session.employeeId && filters.employeeId !== session.employeeId) {
      updateParams({ employee_id: session.employeeId }, true);
    }
  }, [session.role, session.employeeId, filters.employeeId, updateParams]);

  const enriched = useMemo(() => enrichChats(
    chatsQuery.data ?? [],
    contactsQuery.data ?? [],
    accountsQuery.data ?? [],
    employeesQuery.data ?? [],
  ), [chatsQuery.data, contactsQuery.data, accountsQuery.data, employeesQuery.data]);
  const allEnriched = useMemo(() => enrichChats(
    allChatsQuery.data ?? [],
    contactsQuery.data ?? [],
    accountsQuery.data ?? [],
    employeesQuery.data ?? [],
  ), [allChatsQuery.data, contactsQuery.data, accountsQuery.data, employeesQuery.data]);
  const filtered = useMemo(() => filterChats(enriched, {
    ...filters,
    employeeId,
    noReplyThresholdMinutes: companyQuery.data?.noReplyThresholdMinutes,
  }, now), [enriched, filters, employeeId, companyQuery.data?.noReplyThresholdMinutes, now]);
  const selectedItem = allEnriched.find((chat) => chat.id === filters.chatId);
  const outsideFilter = filters.chatId !== undefined && !filtered.some((chat) => chat.id === filters.chatId);
  const hasFilters = Boolean(filters.status || filters.employeeId || filters.accountId || filters.query || filters.unanswered);

  useEffect(() => {
    if (filters.view !== 'split' || filters.chatId || filtered.length === 0) return;
    if (window.matchMedia('(min-width: 768px)').matches) {
      updateParams({ chat_id: filtered[0].id }, true);
    }
  }, [filters.view, filters.chatId, filtered, updateParams]);

  if (!enabled) {
    return <EmptyState icon={UserRoundCheck} title="Выберите сотрудника" description="Для просмотра чатов в роли manager выберите сотрудника в Dev Session сверху" className="h-full" />;
  }

  const resetFilters = () => setSearchParams(filters.view === 'kanban' ? { view: 'kanban' } : {});
  const directoryError = [accountsQuery.error, employeesQuery.error, contactsQuery.error].find(Boolean);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ChatsToolbar
        view={filters.view}
        chats={filtered}
        hasFilters={hasFilters}
        onViewChange={(view) => updateParams({ view: view === 'split' ? undefined : 'kanban' })}
        onReset={resetFilters}
      />
      {directoryError && (
        <div className="flex shrink-0 items-center gap-2 border-b border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-800" role="alert">
          <AlertCircle className="size-4" /> Часть справочников недоступна: {directoryError.message}. Показаны доступные данные.
        </div>
      )}
      {filters.unanswered && companyQuery.isError && (
        <div className="shrink-0 border-b border-red-200 bg-red-50 px-4 py-2 text-xs text-red-700">Не удалось загрузить порог компании: {companyQuery.error.message}. Просроченные чаты не показаны, чтобы не искажать выборку.</div>
      )}
      {filters.view === 'kanban' ? (
        <div className="min-h-0 flex-1">
          <KanbanBoard companyId={session.companyId} chats={filtered} onOpen={(chatId) => updateParams({ view: undefined, chat_id: chatId })} />
        </div>
      ) : (
        <div className="flex min-h-0 min-w-0 flex-1">
          <div className={`${filters.chatId ? 'hidden lg:flex' : 'flex'} h-full w-full shrink-0 border-r border-stone-200 lg:w-[320px] xl:w-[360px]`}>
            <ChatListColumn
              chats={filtered}
              filters={{ ...filters, employeeId }}
              accounts={(accountsQuery.data ?? []).filter((account) => session.role === 'owner' || account.employee_id === session.employeeId)}
              employees={employeesQuery.data ?? []}
              role={session.role}
              thresholdMinutes={companyQuery.data?.noReplyThresholdMinutes}
              isLoading={chatsQuery.isLoading || accountsQuery.isLoading || employeesQuery.isLoading || contactsQuery.isLoading}
              error={chatsQuery.error?.message}
              onRetry={() => void chatsQuery.refetch()}
              onFiltersChange={updateParams}
              onSelect={(chatId) => updateParams({ chat_id: chatId })}
            />
          </div>
          <MessageStreamColumn
            companyId={session.companyId}
            chatId={filters.chatId}
            item={selectedItem}
            outsideFilter={outsideFilter}
            onOpenDetails={() => setDetailsOpen(true)}
            onBack={() => updateParams({ chat_id: undefined })}
          />
          <aside className="hidden h-full w-[360px] shrink-0 border-l border-stone-200 bg-white min-[1440px]:block">
            <ChatRightPanel companyId={session.companyId} chatId={filters.chatId} allChats={allEnriched} />
          </aside>
          <Dialog open={detailsOpen} onOpenChange={setDetailsOpen} title="Детали чата" description="Саммари, контакт и аналитика" side="right">
            <div className="h-full"><ChatRightPanel companyId={session.companyId} chatId={filters.chatId} allChats={allEnriched} /></div>
          </Dialog>
        </div>
      )}
    </div>
  );
}
