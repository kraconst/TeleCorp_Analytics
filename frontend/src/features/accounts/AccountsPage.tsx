import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowRight, CheckCircle2, Clock3, LoaderCircle, Plus, RefreshCw, Send, UserRoundCog, WalletCards } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { Link } from 'react-router-dom';
import { z } from 'zod';
import { assignAccount, createAccount, getAccount } from '@/api/accounts';
import { useAccounts, useEmployees } from '@/api/hooks';
import { queryKeys } from '@/api/queryKeys';
import { ACCOUNT_POLL_INTERVAL, ACCOUNT_POLL_TIMEOUT } from '@/config/env';
import { useToast } from '@/components/feedback/ToastProvider';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { EmptyState, ErrorState, Skeleton } from '@/components/ui/States';
import { useDevSession } from '@/context/DevSessionContext';
import { useSharedNow } from '@/context/NowContext';
import { displayUsername, formatRelativeTime } from '@/lib/formatters';
import { accountStatusConfig, getStatusMeta } from '@/lib/statusConfig';
import type { Account, Employee } from '@/types/api';

const accountSchema = z.object({
  phone: z.string().trim().min(5, 'Введите номер телефона').max(32),
  username: z.string().trim().max(64),
  employee_id: z.number().int().positive('Выберите сотрудника'),
});
type AccountFormValues = z.infer<typeof accountSchema>;

function ConnectionStatus({ companyId, created, startedAt }: { companyId: number; created: Account; startedAt: number }) {
  const queryClient = useQueryClient();
  const now = useSharedNow();
  const query = useQuery({
    queryKey: queryKeys.account(companyId, created.id),
    queryFn: ({ signal }) => getAccount(companyId, created.id, signal),
    initialData: created,
    refetchInterval: (query) => query.state.data?.status === 'connecting' && Date.now() - startedAt < ACCOUNT_POLL_TIMEOUT ? ACCOUNT_POLL_INTERVAL : false,
  });
  const account = query.data ?? created;
  useEffect(() => {
    if (account.status !== 'connecting') void queryClient.invalidateQueries({ queryKey: queryKeys.accounts(companyId) });
  }, [account.status, companyId, queryClient]);
  const timedOut = account.status === 'connecting' && now.getTime() - startedAt >= ACCOUNT_POLL_TIMEOUT;
  return (
    <div className="p-5">
      <div className="rounded-lg border border-stone-200 bg-stone-50 p-5 text-center">
        {account.status === 'active' ? <CheckCircle2 className="mx-auto size-[18px] text-emerald-600" /> : account.status === 'connecting' ? <LoaderCircle className={`mx-auto size-[18px] text-blue-600 ${timedOut ? '' : 'animate-spin'}`} /> : <WalletCards className="mx-auto size-[18px] text-red-600" />}
        <h3 className="mt-3 font-display text-lg font-medium text-stone-900">{account.status === 'active' ? 'Аккаунт активен' : account.status === 'connecting' ? 'Ожидает серверного подключения' : getStatusMeta(accountStatusConfig, account.status, 'Неизвестный статус').label}</h3>
        <p className="mt-2 text-sm leading-6 text-stone-500">
          {account.status === 'connecting'
            ? timedOut ? 'Автоматический статус не получен. Запись сохранена; обновить состояние можно вручную.' : 'Запись создана. Telegram-авторизация должна быть выполнена серверной интеграцией.'
            : `Текущий статус: ${getStatusMeta(accountStatusConfig, account.status, 'Неизвестный статус').label}`}
        </p>
        <p className="mt-3 text-xs font-medium text-stone-700">{displayUsername(account.username)} · {account.phone}</p>
        {(timedOut || account.status !== 'connecting') && <Button variant="secondary" size="sm" className="mt-4" onClick={() => void query.refetch()} disabled={query.isFetching}><RefreshCw className={`size-4 ${query.isFetching ? 'animate-spin' : ''}`} /> Обновить статус</Button>}
      </div>
      <p className="mt-4 text-xs leading-5 text-stone-500">QR-код, ввод кода и 2FA появятся после добавления соответствующего HTTP API. Форма не имитирует эти шаги.</p>
    </div>
  );
}

function CreateAccountDialog({ companyId, employees, open, onOpenChange }: { companyId: number; employees: Employee[]; open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const activeEmployees = employees.filter((employee) => employee.is_active);
  const [created, setCreated] = useState<Account | undefined>();
  const [startedAt, setStartedAt] = useState(0);
  const { register, handleSubmit, reset, formState: { errors } } = useForm<AccountFormValues>({
    resolver: zodResolver(accountSchema),
    defaultValues: { phone: '', username: '', employee_id: activeEmployees[0]?.id ?? 0 },
  });
  const mutation = useMutation({
    mutationFn: (values: AccountFormValues) => createAccount(companyId, values),
    onSuccess: (account) => {
      setCreated(account);
      setStartedAt(Date.now());
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts(companyId) });
      showToast('Запись аккаунта создана');
    },
  });
  const changeOpen = (value: boolean) => {
    onOpenChange(value);
    if (!value) {
      setCreated(undefined);
      reset({ phone: '', username: '', employee_id: activeEmployees[0]?.id ?? 0 });
      mutation.reset();
    }
  };
  return (
    <Dialog open={open} onOpenChange={changeOpen} title={created ? 'Состояние подключения' : 'Добавить Telegram-аккаунт'} description={created ? displayUsername(created.username) : 'Создайте запись и назначьте активного ответственного'}>
      {created ? <ConnectionStatus companyId={companyId} created={created} startedAt={startedAt} /> : (
        <form className="space-y-4 p-5" onSubmit={handleSubmit((values) => mutation.mutate(values))}>
          <label className="block text-xs font-medium text-stone-600">Номер телефона<Input className="mt-2" placeholder="+7 999 123-45-67" autoComplete="tel" {...register('phone')} />{errors.phone && <span className="mt-1 block text-red-600">{errors.phone.message}</span>}</label>
          <label className="block text-xs font-medium text-stone-600">Username <span className="font-normal text-stone-500">(необязательно)</span><Input className="mt-2" placeholder="@sales_anna" {...register('username')} /></label>
          <label className="block text-xs font-medium text-stone-600">Ответственный<Select className="mt-2 w-full" {...register('employee_id', { valueAsNumber: true })}><option value={0} disabled>Выберите сотрудника</option>{activeEmployees.map((employee) => <option key={employee.id} value={employee.id}>{employee.name} · {employee.position}</option>)}</Select>{errors.employee_id && <span className="mt-1 block text-red-600">{errors.employee_id.message}</span>}</label>
          <div className="rounded-lg bg-blue-50 p-3 text-xs leading-5 text-blue-800"><Send className="mr-2 inline size-4" />Аккаунт будет создан со статусом «Подключается». Авторизация Telegram выполняется на сервере.</div>
          {mutation.isError && <ErrorState description={mutation.error.message} />}
          <div className="flex justify-end gap-2 border-t border-stone-200 pt-4"><Button variant="ghost" onClick={() => changeOpen(false)}>Отмена</Button><Button type="submit" disabled={mutation.isPending || activeEmployees.length === 0}>{mutation.isPending ? 'Создаём…' : 'Создать аккаунт'}</Button></div>
        </form>
      )}
    </Dialog>
  );
}

function ReassignDialog({ companyId, account, employees, onClose }: { companyId: number; account?: Account; employees: Employee[]; onClose: () => void }) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const [employeeId, setEmployeeId] = useState('');
  const candidates = employees.filter((employee) => employee.is_active && employee.id !== account?.employee_id);
  const mutation = useMutation({
    mutationFn: () => assignAccount(companyId, account!.id, Number(employeeId)),
    onSuccess: async () => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: queryKeys.accounts(companyId) }), queryClient.invalidateQueries({ queryKey: queryKeys.chatLists(companyId) })]);
      showToast('Ответственный изменён');
      setEmployeeId('');
      onClose();
    },
  });
  return (
    <Dialog open={Boolean(account)} onOpenChange={(open) => { if (!open && !mutation.isPending) onClose(); }} title="Передать аккаунт" description={account ? `${displayUsername(account.username)} · ${account.employee_name}` : undefined}>
      <div className="space-y-4 p-5">
        <label className="block text-xs font-medium text-stone-600">Новый ответственный<Select className="mt-2 w-full" value={employeeId} onChange={(event) => setEmployeeId(event.target.value)}><option value="">Выберите активного сотрудника</option>{candidates.map((employee) => <option key={employee.id} value={employee.id}>{employee.name}</option>)}</Select></label>
        {candidates.length === 0 && <p className="text-xs text-amber-700">Нет других активных сотрудников для передачи</p>}
        {mutation.isError && <ErrorState description={mutation.error.message} />}
        <div className="flex justify-end gap-2"><Button variant="ghost" onClick={onClose}>Отмена</Button><Button disabled={!employeeId || mutation.isPending} onClick={() => mutation.mutate()}>{mutation.isPending ? 'Передаём…' : 'Подтвердить передачу'}</Button></div>
      </div>
    </Dialog>
  );
}

export function AccountsPage() {
  const { session } = useDevSession();
  const accountsQuery = useAccounts(session.companyId);
  const employeesQuery = useEmployees(session.companyId);
  const [createOpen, setCreateOpen] = useState(false);
  const [reassign, setReassign] = useState<Account | undefined>();
  const accounts = (accountsQuery.data ?? []).filter((account) => session.role === 'owner' || account.employee_id === session.employeeId);
  return (
    <div className="mx-auto max-w-6xl space-y-5 p-4 sm:p-6">
      <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div><p className="text-xs font-medium text-blue-600">Источники данных</p><h2 className="mt-1 font-display text-3xl font-medium text-stone-950">{session.role === 'manager' ? 'Мои Telegram-аккаунты' : 'Telegram-аккаунты'}</h2><p className="mt-1 text-sm text-stone-500">Состояние сбора сообщений и ответственные сотрудники</p></div>
        {session.role === 'owner' && <Button onClick={() => setCreateOpen(true)}><Plus className="size-4" /> Добавить аккаунт</Button>}
      </section>
      {accountsQuery.isError ? <ErrorState description={accountsQuery.error.message} onRetry={() => void accountsQuery.refetch()} /> : accountsQuery.isLoading ? <div className="space-y-3"><Skeleton className="h-20" /><Skeleton className="h-20" /></div> : accounts.length === 0 ? <EmptyState icon={WalletCards} title="Аккаунтов пока нет" description={session.role === 'owner' ? 'Добавьте первую запись и назначьте ответственного' : 'За выбранным сотрудником не закреплено аккаунтов'} action={session.role === 'owner' ? <Button onClick={() => setCreateOpen(true)}>Добавить аккаунт</Button> : undefined} /> : (
        <div className="overflow-hidden rounded-xl border border-stone-200 bg-white">
          <div className="hidden grid-cols-[minmax(220px,1.2fr)_minmax(180px,1fr)_130px_150px_auto] gap-4 border-b border-stone-200 bg-stone-50 px-5 py-3 text-xs font-medium text-stone-500 md:grid"><span>Аккаунт</span><span>Ответственный</span><span>Статус</span><span>Активность</span><span>Действия</span></div>
          <ul className="divide-y divide-stone-100">
            {accounts.map((account) => <li key={account.id} className="grid gap-4 p-4 md:grid-cols-[minmax(220px,1.2fr)_minmax(180px,1fr)_130px_150px_auto] md:items-center md:px-5"><div><p className="font-medium text-stone-900">{displayUsername(account.username)}</p><p className="mt-1 text-xs tabular-nums text-stone-500">{account.phone}</p></div><div><p className="text-sm text-stone-700">{account.employee_name || employeesQuery.data?.find((employee) => employee.id === account.employee_id)?.name || 'Сотрудник не найден'}</p><p className="mt-1 text-xs text-stone-500">ID {account.employee_id}</p></div><div><Badge className={getStatusMeta(accountStatusConfig, account.status, 'Неизвестный статус').className}>{getStatusMeta(accountStatusConfig, account.status, 'Неизвестный статус').label}</Badge>{(account.status === 'disconnected' || account.status === 'banned') && <p className="mt-1 text-xs leading-4 text-red-600">Требуется серверное переподключение</p>}</div><p className="flex items-center gap-2 text-xs text-stone-500"><Clock3 className="size-4" /> {formatRelativeTime(account.last_activity_at)}</p><div className="flex flex-wrap gap-2">{session.role === 'owner' && <Button variant="ghost" size="sm" onClick={() => setReassign(account)}><UserRoundCog className="size-4" /> Передать</Button>}<Button asChild variant="secondary" size="sm"><Link to={`/chats?account_id=${account.id}`}>Чаты <ArrowRight className="size-4" /></Link></Button></div></li>)}
          </ul>
        </div>
      )}
      <p className="rounded-lg border border-stone-200 bg-stone-100 px-4 py-3 text-xs leading-5 text-stone-500">Ручная синхронизация и переподключение появятся после соответствующих серверных endpoints. Текущий интерфейс не сообщает ложный успех для этих операций.</p>
      <CreateAccountDialog key={`${session.companyId}-${createOpen}`} companyId={session.companyId} employees={employeesQuery.data ?? []} open={createOpen} onOpenChange={setCreateOpen} />
      <ReassignDialog key={reassign?.id ?? 'closed'} companyId={session.companyId} account={reassign} employees={employeesQuery.data ?? []} onClose={() => setReassign(undefined)} />
    </div>
  );
}
