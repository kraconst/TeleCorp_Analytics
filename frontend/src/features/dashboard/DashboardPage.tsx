import { ArrowDown, ArrowRight, ArrowUp, ArrowUpDown, Download } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useAccountRatings, useAccounts, useAlerts, useChats, useCompany, useDashboard } from '@/api/hooks';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { ErrorState, Skeleton } from '@/components/ui/States';
import { useDevSession } from '@/context/DevSessionContext';
import { downloadCsv } from '@/lib/csv';
import { displayUsername, formatDuration, formatNumber, formatRelativeTime, getDateRange } from '@/lib/formatters';
import { alertTypeConfig, chatStatusConfig, chatStatusOrder, getStatusMeta } from '@/lib/statusConfig';
import type { AccountRating } from '@/types/api';

type SortKey = 'total_messages' | 'messages_in' | 'messages_out' | 'active_chats' | 'new_contacts' | 'avgResponseSeconds';

function validDate(value: string | null): value is string {
  return Boolean(value && /^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(new Date(`${value}T00:00:00`).getTime()));
}

export function DashboardPage() {
  const { session } = useDevSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const defaults = useMemo(() => getDateRange(30), []);
  const from = searchParams.get('from');
  const to = searchParams.get('to');
  const range = { from: validDate(from) ? from : defaults.from, to: validDate(to) ? to : defaults.to };
  const validRange = validDate(from) && validDate(to) && from <= to;
  const dashboard = useDashboard(session.companyId, range, validRange);
  const ratings = useAccountRatings(session.companyId, range, validRange);
  const chats = useChats(session.companyId);
  const accounts = useAccounts(session.companyId);
  const alerts = useAlerts(session.companyId);
  const company = useCompany(session.companyId);
  const [sort, setSort] = useState<{ key: SortKey; direction: 'asc' | 'desc' }>({ key: 'total_messages', direction: 'desc' });

  useEffect(() => {
    if (!from || !to) {
      const next = new URLSearchParams(searchParams);
      next.set('from', range.from);
      next.set('to', range.to);
      setSearchParams(next, { replace: true });
    }
  }, [from, to, range.from, range.to, searchParams, setSearchParams]);

  const accountMap = new Map((accounts.data ?? []).map((account) => [account.id, account]));
  const chatMap = new Map((chats.data ?? []).map((chat) => [chat.id, chat]));
  const sortedRatings = [...(ratings.data ?? [])].sort((a, b) => {
    const result = a[sort.key] - b[sort.key];
    return sort.direction === 'asc' ? result : -result;
  });
  const maxStatusCount = Math.max(1, ...chatStatusOrder.map((status) => (chats.data ?? []).filter((chat) => chat.status === status).length));
  const updateRange = (fromValue: string, toValue: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('from', fromValue);
    next.set('to', toValue);
    setSearchParams(next);
  };
  const sortBy = (key: SortKey) => setSort((current) => ({ key, direction: current.key === key && current.direction === 'desc' ? 'asc' : 'desc' }));
  const sortIcon = (key: SortKey) => sort.key !== key ? <ArrowUpDown className="size-4" /> : sort.direction === 'asc' ? <ArrowUp className="size-4" /> : <ArrowDown className="size-4" />;
  const stats = dashboard.data;
  const kpis = [
    { label: 'Входящие', value: stats ? formatNumber(stats.messages_in) : '—', note: 'за период' },
    { label: 'Исходящие', value: stats ? formatNumber(stats.messages_out) : '—', note: 'за период' },
    { label: 'Активные чаты', value: stats ? formatNumber(stats.active_chats) : '—', note: 'за период' },
    { label: 'Новые клиенты', value: stats ? formatNumber(stats.new_contacts) : '—', note: 'за период' },
    { label: 'Средний ответ', value: stats ? formatDuration(stats.avgResponseSeconds) : '—', note: 'за период' },
  ];

  const exportRatings = () => downloadCsv(
    `telecorp-rating-${range.from}-${range.to}.csv`,
    ['Аккаунт', 'Сотрудник', 'Входящие', 'Исходящие', 'Всего', 'Активные чаты', 'Новые клиенты', 'Средний ответ, сек'],
    sortedRatings.map((rating) => [displayUsername(rating.username), rating.employee_name, rating.messages_in, rating.messages_out, rating.total_messages, rating.active_chats, rating.new_contacts, rating.avgResponseSeconds]),
  );

  return (
    <div className="mx-auto max-w-[1600px] space-y-6 p-4 sm:p-6">
      <section className="flex flex-col justify-between gap-4 xl:flex-row xl:items-end">
        <div>
          <p className="text-xs font-medium text-blue-600">Рабочая сводка</p>
          <h2 className="mt-1 font-display text-3xl font-medium text-stone-950">Состояние отдела</h2>
          <p className="mt-1 text-sm text-stone-500">Метрики за период и текущие риски — с переходом к конкретным диалогам</p>
        </div>
        <div className="flex flex-wrap items-end gap-2 rounded-xl border border-stone-200 bg-white p-1">
          <div className="flex gap-1">
            {[7, 30, 90].map((days) => <Button key={days} variant={from === getDateRange(days).from && to === getDateRange(days).to ? 'secondary' : 'ghost'} size="sm" onClick={() => { const next = getDateRange(days); updateRange(next.from, next.to); }}>{days} дней</Button>)}
          </div>
          <label className="text-xs text-stone-500">С<Input type="date" className="mt-0.5 h-8 w-[136px] text-xs" value={range.from} onChange={(event) => updateRange(event.target.value, range.to)} /></label>
          <label className="text-xs text-stone-500">По<Input type="date" className="mt-0.5 h-8 w-[136px] text-xs" value={range.to} onChange={(event) => updateRange(range.from, event.target.value)} /></label>
        </div>
      </section>
      {!validRange && <ErrorState title="Некорректный период" description="Проверьте даты: начало периода должно быть не позже его окончания" />}
      {dashboard.isError && <ErrorState description={dashboard.error.message} onRetry={() => void dashboard.refetch()} />}
      <section className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6" aria-label="Ключевые показатели">
        {kpis.map((kpi) => (
          <article key={kpi.label} className="rounded-xl border border-stone-200 bg-white p-4">
            <p className="text-xs font-medium text-stone-500">{kpi.label}</p>
            {dashboard.isLoading ? <Skeleton className="mt-3 h-7 w-20" /> : <p className="mt-2 font-display text-3xl font-medium tabular-nums text-stone-950">{kpi.value}</p>}
            <p className="mt-1 text-xs text-stone-500">{kpi.note}</p>
          </article>
        ))}
        <Link to="/chats?unanswered=true" className={`group rounded-xl border p-4 transition-colors ${stats?.unanswered_chats ? 'border-red-200 bg-red-50 hover:border-red-300' : 'border-stone-200 bg-white hover:border-stone-300'}`}>
          <div className="flex items-center justify-between"><p className={`text-xs font-medium ${stats?.unanswered_chats ? 'text-red-700' : 'text-stone-500'}`}>Чаты без ответа</p><ArrowRight className={`size-4 ${stats?.unanswered_chats ? 'text-red-600' : 'text-stone-500'}`} /></div>
          {dashboard.isLoading ? <Skeleton className="mt-3 h-7 w-20" /> : <p className={`mt-2 font-display text-3xl font-medium tabular-nums ${stats?.unanswered_chats ? 'text-red-800' : 'text-stone-950'}`}>{stats?.unanswered_chats ?? 0}</p>}
          <p className={`mt-1 text-xs ${stats?.unanswered_chats ? 'text-red-600' : 'text-stone-500'}`}>сейчас · открыть очередь</p>
        </Link>
      </section>
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.55fr)_minmax(320px,0.8fr)]">
        <section className="rounded-xl border border-stone-200 bg-white">
          <header className="flex items-center justify-between border-b border-stone-200 px-5 py-4">
            <div><h3 className="font-display text-sm font-medium text-stone-900">Рейтинг аккаунтов</h3><p className="mt-0.5 text-xs text-stone-500">Активность за выбранный период</p></div>
            <Button variant="ghost" size="sm" onClick={exportRatings} disabled={!sortedRatings.length}><Download className="size-4" /> CSV</Button>
          </header>
          {ratings.isError ? <ErrorState className="m-4" description={ratings.error.message} onRetry={() => void ratings.refetch()} /> : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-xs">
                <thead className="bg-stone-50 text-xs text-stone-500"><tr><th className="px-5 py-3 font-medium">Аккаунт / менеджер</th>{([['total_messages', 'Всего'], ['messages_in', 'Входящие'], ['messages_out', 'Исходящие'], ['active_chats', 'Чаты'], ['new_contacts', 'Клиенты'], ['avgResponseSeconds', 'Ответ']] as [SortKey, string][]).map(([key, label]) => <th key={key} className="px-3 py-3 font-medium"><button className="flex cursor-pointer items-center gap-1 hover:text-stone-700" onClick={() => sortBy(key)}>{label}{sortIcon(key)}</button></th>)}</tr></thead>
                <tbody className="divide-y divide-stone-100">
                  {ratings.isLoading ? Array.from({ length: 3 }, (_, index) => <tr key={index}><td colSpan={7} className="px-5 py-3"><Skeleton className="h-10" /></td></tr>) : sortedRatings.map((rating: AccountRating) => {
                    const account = accountMap.get(rating.account_id);
                    return <tr key={rating.account_id} className="hover:bg-stone-50/70"><td className="px-5 py-3"><Link className="font-medium text-blue-700 hover:underline" to={`/chats?account_id=${rating.account_id}`}>{displayUsername(rating.username)}</Link><p className="mt-0.5 text-xs text-stone-500">{rating.employee_name || 'Сотрудник не найден'}{account?.status === 'disconnected' || account?.status === 'banned' ? ' · сбор остановлен' : ''}</p></td><td className="px-3 py-3 font-medium tabular-nums">{formatNumber(rating.total_messages)}</td><td className="px-3 py-3 tabular-nums">{formatNumber(rating.messages_in)}</td><td className="px-3 py-3 tabular-nums">{formatNumber(rating.messages_out)}</td><td className="px-3 py-3 tabular-nums">{rating.active_chats}</td><td className="px-3 py-3 tabular-nums">{rating.new_contacts}</td><td className="px-3 py-3 tabular-nums">{formatDuration(rating.avgResponseSeconds)}</td></tr>;
                  })}
                </tbody>
              </table>
              {!ratings.isLoading && sortedRatings.length === 0 && <p className="p-8 text-center text-sm text-stone-500">За этот период нет данных рейтинга</p>}
            </div>
          )}
        </section>
        <div className="space-y-6">
          <section className="rounded-xl border border-stone-200 bg-white">
            <header className="border-b border-stone-200 px-5 py-4"><h3 className="font-display text-sm font-medium text-stone-900">Статусы чатов</h3><p className="mt-0.5 text-xs text-stone-500">Текущий срез, не когортная конверсия</p></header>
            <div className="space-y-3 p-5">
              {chatStatusOrder.map((status) => {
                const count = (chats.data ?? []).filter((chat) => chat.status === status).length;
                return <Link key={status} to={`/chats?status=${status}`} className="group block"><div className="mb-1 flex items-center justify-between gap-3 text-xs"><span className="text-stone-600 group-hover:text-blue-700">{chatStatusConfig[status].label}</span><span className="font-medium tabular-nums text-stone-700">{count}</span></div><div className="h-1.5 overflow-hidden rounded-full bg-stone-100"><div className={`h-full rounded-full ${status === 'action_required' ? 'bg-red-400' : status === 'lost' ? 'bg-stone-400' : 'bg-blue-400'}`} style={{ width: `${count ? Math.max(4, count / maxStatusCount * 100) : 0}%` }} /></div></Link>;
              })}
            </div>
          </section>
          <section className="rounded-xl border border-stone-200 bg-white">
            <header className="border-b border-stone-200 px-5 py-4"><h3 className="font-display text-sm font-medium text-stone-900">Свежие алерты</h3><p className="mt-0.5 text-xs text-stone-500">Три последних активных</p></header>
            <div className="divide-y divide-stone-100">
              {alerts.isError ? <ErrorState className="m-3" description={alerts.error.message} /> : alerts.data?.slice(0, 3).map((alert) => <Link key={alert.id} to={chatMap.has(alert.chat_id) ? `/chats?view=split&chat_id=${alert.chat_id}` : '/alerts'} className="block p-4 hover:bg-stone-50"><Badge className={getStatusMeta(alertTypeConfig, alert.type, 'Неизвестный алерт').className}>{getStatusMeta(alertTypeConfig, alert.type, 'Неизвестный алерт').label}</Badge><p className="mt-2 text-xs font-medium text-stone-800">Чат #{alert.chat_id}</p><p className="mt-1 text-xs text-stone-500">{formatRelativeTime(alert.created_at)}</p></Link>)}
              {!alerts.isLoading && !alerts.isError && alerts.data?.length === 0 && <p className="p-6 text-center text-xs text-stone-500">Активных алертов нет</p>}
            </div>
            <div className="border-t border-stone-100 p-3"><Button asChild variant="ghost" size="sm" className="w-full"><Link to="/alerts">Вся очередь <ArrowRight className="size-4" /></Link></Button></div>
          </section>
        </div>
      </div>
      <p className="text-xs text-stone-500">{company.data?.name ?? `Компания #${session.companyId}`} · часовой пояс {company.data?.timezone || 'UTC'}</p>
    </div>
  );
}
