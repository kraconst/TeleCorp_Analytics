import { useMutation, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, ArrowRight, BellRing, Check, CheckCheck, Clock3, Inbox } from 'lucide-react';
import { Link, useSearchParams } from 'react-router-dom';
import { resolveAlert } from '@/api/alerts';
import { useAccounts, useAlerts, useChats, useContacts, useEmployees } from '@/api/hooks';
import { queryKeys } from '@/api/queryKeys';
import { useToast } from '@/components/feedback/ToastProvider';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Select } from '@/components/ui/Select';
import { EmptyState, ErrorState, Skeleton } from '@/components/ui/States';
import { useDevSession } from '@/context/DevSessionContext';
import { useResolvedAlerts, type SessionResolvedAlert } from '@/context/ResolvedAlertsContext';
import { useSharedNow } from '@/context/NowContext';
import { enrichChats, contactDisplayName } from '@/lib/chatFilters';
import { displayUsername, formatRelativeTime } from '@/lib/formatters';
import { alertTypeConfig, chatStatusConfig, getStatusMeta, isAlertType } from '@/lib/statusConfig';
import type { Alert, AlertType, ChatListItem } from '@/types/api';

function AlertCard({ alert, chat, resolvedAt, onResolve, isResolving }: {
  alert: Alert;
  chat?: ChatListItem;
  resolvedAt?: string;
  onResolve?: () => void;
  isResolving?: boolean;
}) {
  const now = useSharedNow();
  const type = getStatusMeta(alertTypeConfig, alert.type, 'Неизвестный алерт');
  return (
    <article className="rounded-xl border border-stone-200 bg-white p-4 transition-colors hover:border-stone-300">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <span className={`mt-0.5 rounded-lg border p-2 ${type.className}`}><AlertTriangle className="size-4" /></span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge className={type.className}>{type.label}</Badge>
            <span className="text-xs tabular-nums text-stone-500">Алерт #{alert.id}</span>
            {resolvedAt && <Badge className="border-emerald-200 bg-emerald-50 text-emerald-700"><CheckCheck className="size-4" /> Обработан</Badge>}
          </div>
          <h2 className="mt-2 text-sm font-medium text-stone-900">{chat ? contactDisplayName(chat) : `Чат #${alert.chat_id}`}</h2>
          <p className="mt-1 text-xs leading-5 text-stone-500">{type.description}</p>
          <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-stone-500">
            <span className="flex items-center gap-1"><Clock3 className="size-4" /> {formatRelativeTime(resolvedAt ?? alert.created_at, now)}</span>
            {chat && <span>{chat.employee?.name || chat.account?.employee_name || 'Сотрудник не найден'} · {displayUsername(chat.account?.username)}</span>}
            {chat && <Badge className={getStatusMeta(chatStatusConfig, chat.status, 'Неизвестный статус').className}>{getStatusMeta(chatStatusConfig, chat.status, 'Неизвестный статус').label}</Badge>}
          </div>
        </div>
        <div className="flex shrink-0 gap-2 sm:self-center">
          {chat && <Button asChild variant="secondary" size="sm"><Link to={`/chats?view=split&chat_id=${chat.id}`}>Открыть чат <ArrowRight className="size-4" /></Link></Button>}
          {!resolvedAt && onResolve && <Button variant="subtle" size="sm" onClick={onResolve} disabled={isResolving}><Check className="size-4" /> {isResolving ? 'Закрываем…' : 'Обработано'}</Button>}
        </div>
      </div>
    </article>
  );
}

export function AlertsPage() {
  const { session } = useDevSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { resolved, addResolved } = useResolvedAlerts();
  const employeeId = session.role === 'manager' ? (session.employeeId ?? undefined) : undefined;
  const alertsQuery = useAlerts(session.companyId);
  const chatsQuery = useChats(session.companyId, undefined, employeeId, session.role === 'owner' || Boolean(employeeId));
  const contacts = useContacts(session.companyId);
  const accounts = useAccounts(session.companyId);
  const employees = useEmployees(session.companyId);
  const enriched = enrichChats(chatsQuery.data ?? [], contacts.data ?? [], accounts.data ?? [], employees.data ?? []);
  const chatMap = new Map(enriched.map((chat) => [chat.id, chat]));
  const selectedTypeValue = searchParams.get('type');
  const selectedType: AlertType | undefined = isAlertType(selectedTypeValue) ? selectedTypeValue : undefined;
  const tab = searchParams.get('tab') === 'resolved' ? 'resolved' : 'active';
  const activeAlerts = (alertsQuery.data ?? []).filter((alert) => {
    if (alert.company_id !== session.companyId) return false;
    if (session.role === 'manager' && !chatMap.has(alert.chat_id)) return false;
    return !selectedType || alert.type === selectedType;
  });
  const sessionResolved = resolved.filter((item) => {
    if (item.alert.company_id !== session.companyId) return false;
    if (session.role === 'manager' && !chatMap.has(item.alert.chat_id)) return false;
    return !selectedType || item.alert.type === selectedType;
  });

  const mutation = useMutation({
    mutationFn: ({ alert }: { alert: Alert }) => resolveAlert(session.companyId, alert.id),
    onMutate: async ({ alert }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.alerts(session.companyId) });
      const previous = queryClient.getQueryData<Alert[]>(queryKeys.alerts(session.companyId));
      queryClient.setQueryData<Alert[]>(queryKeys.alerts(session.companyId), (old) => old?.filter((item) => item.id !== alert.id) ?? []);
      return { previous };
    },
    onError: (error, _variables, context) => {
      queryClient.setQueryData(queryKeys.alerts(session.companyId), context?.previous);
      showToast(`Алерт не закрыт: ${error.message}`, 'error');
    },
    onSuccess: (_data, { alert }) => {
      addResolved(alert);
      showToast('Алерт отмечен как обработанный');
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.alerts(session.companyId) }),
  });

  const setParam = (key: string, value?: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value); else next.delete(key);
    setSearchParams(next);
  };
  const currentResolved: SessionResolvedAlert[] = sessionResolved;

  return (
    <div className="mx-auto max-w-6xl space-y-5 p-4 sm:p-6">
      <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div>
          <p className="text-xs font-medium text-blue-600">Операционная очередь</p>
          <h2 className="mt-1 font-display text-3xl font-medium text-stone-950">Требуют решения</h2>
          <p className="mt-1 text-sm text-stone-500">Не журнал событий, а конкретные диалоги, которым нужно внимание</p>
        </div>
        <Select aria-label="Фильтр по типу алерта" value={selectedType ?? ''} onChange={(event) => setParam('type', event.target.value || undefined)}>
          <option value="">Все типы</option>
          {Object.entries(alertTypeConfig).map(([type, config]) => <option key={type} value={type}>{config.label}</option>)}
        </Select>
      </section>
      <div className="flex items-center gap-1 border-b border-stone-200">
        <button className={`cursor-pointer border-b-2 px-4 py-3 text-sm font-medium ${tab === 'active' ? 'border-blue-600 text-blue-700' : 'border-transparent text-stone-500'}`} onClick={() => setParam('tab', undefined)}>Активные <span className="ml-1 tabular-nums">{activeAlerts.length}</span></button>
        <button className={`cursor-pointer border-b-2 px-4 py-3 text-sm font-medium ${tab === 'resolved' ? 'border-blue-600 text-blue-700' : 'border-transparent text-stone-500'}`} onClick={() => setParam('tab', 'resolved')}>Обработанные в сессии <span className="ml-1 tabular-nums">{sessionResolved.length}</span></button>
      </div>
      {tab === 'resolved' && <p className="rounded-lg border border-stone-200 bg-stone-100 px-4 py-3 text-xs leading-5 text-stone-500">Этот список хранится только в текущей вкладке до перезагрузки. Серверный архив закрытых алертов пока не доступен.</p>}
      {tab === 'active' && alertsQuery.isError ? (
        <ErrorState description={alertsQuery.error.message} onRetry={() => void alertsQuery.refetch()} />
      ) : tab === 'active' && (alertsQuery.isLoading || chatsQuery.isLoading) ? (
        <div className="space-y-3">{Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-32" />)}</div>
      ) : tab === 'active' && activeAlerts.length === 0 ? (
        <EmptyState icon={Inbox} title="Активных алертов нет" description={selectedType ? 'По выбранному типу проблем не найдено. Измените фильтр.' : 'Очередь разобрана. Новые риски появятся здесь после их обнаружения сервером.'} />
      ) : tab === 'resolved' && currentResolved.length === 0 ? (
        <EmptyState icon={CheckCheck} title="В этой сессии пока ничего не закрыто" description="Обработанные алерты появятся здесь и останутся доступны до перезагрузки страницы" />
      ) : (
        <div className="space-y-3">
          {tab === 'active' ? activeAlerts.map((alert) => (
            <AlertCard key={alert.id} alert={alert} chat={chatMap.get(alert.chat_id)} onResolve={() => mutation.mutate({ alert })} isResolving={mutation.isPending && mutation.variables?.alert.id === alert.id} />
          )) : currentResolved.map(({ alert, resolvedAt }) => <AlertCard key={alert.id} alert={alert} chat={chatMap.get(alert.chat_id)} resolvedAt={resolvedAt} />)}
        </div>
      )}
      {session.role === 'manager' && !employeeId && <p className="flex items-center gap-2 text-xs text-amber-700"><BellRing className="size-4" /> Выберите сотрудника в Dev Session, чтобы увидеть только его алерты</p>}
    </div>
  );
}
