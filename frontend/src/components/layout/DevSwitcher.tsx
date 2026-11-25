import { Building2, ChevronDown, FlaskConical } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useEmployees } from '@/api/hooks';
import { useDevSession } from '@/context/DevSessionContext';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';

export function DevSwitcher() {
  const { session, updateSession } = useDevSession();
  const employees = useEmployees(session.companyId);
  const [companyDraft, setCompanyDraft] = useState(String(session.companyId));
  const activeEmployees = (employees.data ?? []).filter((employee) => employee.is_active);

  const changeRole = (role: 'owner' | 'manager') => {
    if (role === 'owner') {
      updateSession({ role, employeeId: null });
      return;
    }
    const employeeId = activeEmployees.some((employee) => employee.id === session.employeeId)
      ? session.employeeId
      : (activeEmployees[0]?.id ?? null);
    updateSession({ role, employeeId });
  };

  const applyCompany = (event: FormEvent) => {
    event.preventDefault();
    const companyId = Number(companyDraft);
    if (Number.isSafeInteger(companyId) && companyId > 0) {
      updateSession({ companyId, role: 'owner', employeeId: null });
    }
  };

  return (
    <div className="flex items-center gap-2 rounded-xl border border-dashed border-amber-300 bg-amber-50 px-2 py-2 text-xs text-amber-950">
      <FlaskConical className="hidden size-4 shrink-0 sm:block" aria-hidden="true" />
      <span className="hidden font-medium 2xl:inline" title="Переключатель влияет только на интерфейс и не заменяет серверную авторизацию">
        Dev Session
      </span>
      <Select
        aria-label="Роль Dev Session"
        className="h-7 w-[94px] border-amber-200 bg-white/80 pl-2 text-xs"
        value={session.role}
        onChange={(event) => changeRole(event.target.value as 'owner' | 'manager')}
      >
        <option value="owner">Owner</option>
        <option value="manager">Manager</option>
      </Select>
      {session.role === 'manager' && (
        <span className="relative">
          <Select
            aria-label="Сотрудник Dev Session"
            className="h-7 max-w-32 border-amber-200 bg-white/80 pl-2 text-xs"
            value={session.employeeId ?? ''}
            disabled={employees.isLoading || activeEmployees.length === 0}
            onChange={(event) => updateSession({ employeeId: Number(event.target.value) })}
          >
            {!session.employeeId && <option value="">Сотрудник</option>}
            {activeEmployees.map((employee) => (
              <option key={employee.id} value={employee.id}>{employee.name}</option>
            ))}
          </Select>
        </span>
      )}
      <form onSubmit={applyCompany} className="hidden items-center gap-1 lg:flex">
        <Building2 className="size-4 text-amber-700" aria-hidden="true" />
        <Input
          aria-label="ID компании"
          className="h-7 w-14 border-amber-200 bg-white/80 px-2 text-center text-xs tabular-nums"
          value={companyDraft}
          onChange={(event) => setCompanyDraft(event.target.value.replace(/[^0-9]/g, ''))}
          inputMode="numeric"
        />
        <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" type="submit" title="Применить company ID">
          <ChevronDown className="size-4 -rotate-90" aria-hidden="true" />
          Применить
        </Button>
      </form>
    </div>
  );
}
