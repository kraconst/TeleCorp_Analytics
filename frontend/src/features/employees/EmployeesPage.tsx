import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, Pencil, Plus, UserRoundX, UsersRound } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { assignAccount } from '@/api/accounts';
import { useAccounts, useEmployees } from '@/api/hooks';
import { createEmployee, deactivateEmployee, updateEmployee } from '@/api/employees';
import { queryKeys } from '@/api/queryKeys';
import { useToast } from '@/components/feedback/ToastProvider';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { EmptyState, ErrorState, Skeleton } from '@/components/ui/States';
import { useDevSession } from '@/context/DevSessionContext';
import { displayUsername } from '@/lib/formatters';
import type { Account, Employee } from '@/types/api';

const employeeSchema = z.object({ name: z.string().trim().min(2, 'Введите имя сотрудника').max(200), position: z.string().trim().max(200) });
type EmployeeFormValues = z.infer<typeof employeeSchema>;

function EmployeeFormDialog({ companyId, employee, employees, open, onOpenChange }: { companyId: number; employee?: Employee; employees: Employee[]; open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { register, handleSubmit, reset, formState: { errors } } = useForm<EmployeeFormValues>({ resolver: zodResolver(employeeSchema), defaultValues: { name: employee?.name ?? '', position: employee?.position ?? '' } });
  const mutation = useMutation<Employee | { status: string }, Error, EmployeeFormValues>({
    mutationFn: (values: EmployeeFormValues) => employee ? updateEmployee(companyId, employee.id, values) : createEmployee(companyId, values),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.employees(companyId) });
      showToast(employee ? 'Сотрудник обновлён' : 'Сотрудник создан');
      onOpenChange(false);
    },
  });
  const changeOpen = (value: boolean) => {
    onOpenChange(value);
    if (!value) {
      reset({ name: '', position: '' });
      mutation.reset();
    }
  };
  return (
    <Dialog open={open} onOpenChange={changeOpen} title={employee ? 'Редактировать сотрудника' : 'Добавить сотрудника'} description="Имя и должность используются в чатах, аккаунтах и отчётах">
      <form className="space-y-4 p-5" onSubmit={handleSubmit((values) => mutation.mutate(values))}>
        <label className="block text-xs font-medium text-stone-600">Имя и фамилия<Input className="mt-2" placeholder="Анна Ковалёва" autoComplete="name" {...register('name')} />{errors.name && <span className="mt-1 block text-red-600">{errors.name.message}</span>}</label>
        <label className="block text-xs font-medium text-stone-600">Должность<Input className="mt-2" placeholder="Менеджер по продажам" {...register('position')} /></label>
        {mutation.isError && <ErrorState description={mutation.error.message} />}
        <div className="flex justify-end gap-2 border-t border-stone-200 pt-4"><Button variant="ghost" onClick={() => changeOpen(false)}>Отмена</Button><Button type="submit" disabled={mutation.isPending}>{mutation.isPending ? 'Сохраняем…' : 'Сохранить'}</Button></div>
      </form>
      <span className="sr-only">Всего сотрудников: {employees.length}</span>
    </Dialog>
  );
}

function DeactivateDialog({ companyId, employee, accounts, employees, onClose }: { companyId: number; employee: Employee; accounts: Account[]; employees: Employee[]; onClose: () => void }) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const linked = accounts.filter((account) => account.employee_id === employee.id);
  const candidates = employees.filter((item) => item.is_active && item.id !== employee.id);
  const [selections, setSelections] = useState<Record<number, string>>(() => Object.fromEntries(linked.map((account) => [account.id, candidates[0] ? String(candidates[0].id) : ''])));
  const [results, setResults] = useState<Record<number, 'success' | 'error'>>({});
  const [currentAccount, setCurrentAccount] = useState<number | undefined>();
  const mutation = useMutation({
    mutationFn: async () => {
      setResults({});
      for (const account of linked) {
        const target = Number(selections[account.id]);
        if (!target) throw new Error(`Выберите нового ответственного для ${displayUsername(account.username)}`);
        setCurrentAccount(account.id);
        try {
          await assignAccount(companyId, account.id, target);
          setResults((current) => ({ ...current, [account.id]: 'success' }));
        } catch (error) {
          setResults((current) => ({ ...current, [account.id]: 'error' }));
          throw new Error(`${displayUsername(account.username)} не передан: ${error instanceof Error ? error.message : 'ошибка сервера'}. Сотрудник не деактивирован.`);
        }
      }
      setCurrentAccount(undefined);
      await deactivateEmployee(companyId, employee.id);
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.employees(companyId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.accounts(companyId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.chatLists(companyId) }),
      ]);
      showToast('Сотрудник деактивирован; история сохранена');
      onClose();
    },
  });
  return (
    <Dialog open onOpenChange={(open) => { if (!open && !mutation.isPending) onClose(); }} title={`Деактивировать: ${employee.name}`} description="История переписок и аналитика сохранится">
      <div className="space-y-4 p-5">
        {linked.length > 0 ? <>
          <p className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-900">Сначала передайте все аккаунты активным сотрудникам. Если одна из операций не выполнится, деактивация не будет вызвана.</p>
          <div className="space-y-3">{linked.map((account) => <div key={account.id} className="rounded-lg border border-stone-200 p-3"><div className="flex items-center justify-between gap-2"><p className="text-sm font-medium text-stone-800">{displayUsername(account.username)}</p>{results[account.id] === 'success' && <Badge className="border-emerald-200 bg-emerald-50 text-emerald-700"><Check className="size-4" /> Передан</Badge>}</div><Select className="mt-2 w-full" value={selections[account.id] ?? ''} disabled={mutation.isPending || results[account.id] === 'success'} onChange={(event) => setSelections((current) => ({ ...current, [account.id]: event.target.value }))}><option value="">Выберите сотрудника</option>{candidates.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.name}</option>)}</Select>{currentAccount === account.id && mutation.isPending && <p className="mt-2 text-xs text-blue-600">Передаём аккаунт…</p>}{results[account.id] === 'error' && <p className="mt-2 text-xs text-red-600">Операция не выполнена. Проверьте данные и повторите.</p>}</div>)}</div>
        </> : <p className="text-sm leading-6 text-stone-600">За сотрудником не закреплены аккаунты. Деактивацию можно выполнить сразу.</p>}
        {mutation.isError && <ErrorState title="Операция остановлена" description={mutation.error.message} />}
        {Object.values(results).includes('success') && mutation.isError && <p className="text-xs leading-5 text-amber-700">Некоторые аккаунты уже переданы. После повторного открытия список обновится; уже успешные операции не нужно откатывать.</p>}
        <div className="flex justify-end gap-2 border-t border-stone-200 pt-4"><Button variant="ghost" onClick={onClose} disabled={mutation.isPending}>Отмена</Button><Button variant="danger" disabled={mutation.isPending || (linked.length > 0 && candidates.length === 0)} onClick={() => mutation.mutate()}><UserRoundX className="size-4" /> {mutation.isPending ? 'Выполняем…' : linked.length ? 'Передать и деактивировать' : 'Деактивировать'}</Button></div>
      </div>
    </Dialog>
  );
}

export function EmployeesPage() {
  const { session } = useDevSession();
  const { showToast } = useToast();
  const employeesQuery = useEmployees(session.companyId);
  const accountsQuery = useAccounts(session.companyId);
  const [showArchived, setShowArchived] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Employee | undefined>();
  const [deactivating, setDeactivating] = useState<Employee | undefined>();
  const [deactivationAccounts, setDeactivationAccounts] = useState<Account[]>([]);
  const [deactivationBusyId, setDeactivationBusyId] = useState<number | undefined>();
  const employees = (employeesQuery.data ?? []).filter((employee) => showArchived || employee.is_active);
  const prepareDeactivation = async (employee: Employee) => {
    setDeactivationBusyId(employee.id);
    try {
      const freshAccounts = await accountsQuery.refetch();
      if (freshAccounts.error) throw freshAccounts.error;
      setDeactivationAccounts(freshAccounts.data ?? []);
      setDeactivating(employee);
    } catch (error) {
      showToast(`Не удалось проверить актуальные аккаунты: ${error instanceof Error ? error.message : 'ошибка сервера'}`, 'error');
    } finally {
      setDeactivationBusyId(undefined);
    }
  };
  return (
    <div className="mx-auto max-w-6xl space-y-5 p-4 sm:p-6">
      <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end"><div><p className="text-xs font-medium text-blue-600">Команда</p><h2 className="mt-1 font-display text-3xl font-medium text-stone-950">Сотрудники</h2><p className="mt-1 text-sm text-stone-500">Ответственные за аккаунты и рабочие диалоги</p></div><Button onClick={() => { setEditing(undefined); setFormOpen(true); }}><Plus className="size-4" /> Добавить сотрудника</Button></section>
      <label className="flex w-fit cursor-pointer items-center gap-2 text-xs text-stone-600"><input type="checkbox" className="size-4 accent-blue-600" checked={showArchived} onChange={(event) => setShowArchived(event.target.checked)} /> Показать деактивированных</label>
      {employeesQuery.isError ? <ErrorState description={employeesQuery.error.message} onRetry={() => void employeesQuery.refetch()} /> : employeesQuery.isLoading ? <Skeleton className="h-64" /> : employees.length === 0 ? <EmptyState icon={UsersRound} title="Сотрудники не найдены" description={showArchived ? 'Добавьте первого сотрудника' : 'Активных сотрудников нет. Включите отображение архива или добавьте нового.'} /> : (
        <div className="overflow-hidden rounded-xl border border-stone-200 bg-white"><ul className="divide-y divide-stone-100">{employees.map((employee) => {
          const linked = (accountsQuery.data ?? []).filter((account) => account.employee_id === employee.id);
          return <li key={employee.id} className="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:px-5"><div className="flex min-w-0 flex-1 items-center gap-3"><span className="grid size-10 shrink-0 place-items-center rounded-lg bg-stone-100 font-display text-sm font-medium text-stone-600">{employee.name.slice(0, 1).toUpperCase()}</span><div className="min-w-0"><p className="truncate text-sm font-medium text-stone-900">{employee.name}</p><p className="mt-0.5 truncate text-xs text-stone-500">{employee.position || 'Должность не указана'}</p></div></div><div className="flex items-center gap-2"><Badge className={employee.is_active ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-stone-200 bg-stone-100 text-stone-500'}>{employee.is_active ? 'Активен' : 'Деактивирован'}</Badge><span className="text-xs tabular-nums text-stone-500">{linked.length} аккаунтов</span></div><div className="flex gap-2"><Button variant="ghost" size="sm" onClick={() => { setEditing(employee); setFormOpen(true); }}><Pencil className="size-4" /> Изменить</Button>{employee.is_active && <Button variant="ghost" size="sm" className="text-red-600 hover:bg-red-50 hover:text-red-700" onClick={() => void prepareDeactivation(employee)} disabled={deactivationBusyId === employee.id}><UserRoundX className="size-4" /> Деактивировать</Button>}</div></li>;
        })}</ul></div>
      )}
      <p className="text-xs text-stone-500">Переписки и аналитика остаются в истории после деактивации</p>
      <EmployeeFormDialog key={`${editing?.id ?? 'new'}-${formOpen}`} companyId={session.companyId} employee={editing} employees={employeesQuery.data ?? []} open={formOpen} onOpenChange={setFormOpen} />
      {deactivating && <DeactivateDialog key={deactivating.id} companyId={session.companyId} employee={deactivating} accounts={deactivationAccounts} employees={employeesQuery.data ?? []} onClose={() => { setDeactivating(undefined); setDeactivationAccounts([]); void accountsQuery.refetch(); }} />}
    </div>
  );
}
