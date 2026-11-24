import {
  BellRing,
  LayoutDashboard,
  Menu,
  MessagesSquare,
  Settings,
  ShieldCheck,
  UsersRound,
  WalletCards,
  X,
} from 'lucide-react';
import { useState, Suspense } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useAccounts, useAlerts, useChats, useCompany } from '@/api/hooks';
import { useDevSession } from '@/context/DevSessionContext';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/Button';
import { LoadingState } from '@/components/ui/States';
import { CompanySettingsDialog } from '@/features/company/CompanySettingsDialog';
import { DevSwitcher } from './DevSwitcher';
import { DisconnectedAccountsBanner } from './DisconnectedAccountsBanner';

const navigation = [
  { to: '/chats', label: 'Чаты', icon: MessagesSquare, roles: ['owner', 'manager'] },
  { to: '/dashboard', label: 'Дашборд', icon: LayoutDashboard, roles: ['owner'] },
  { to: '/alerts', label: 'Алерты', icon: BellRing, roles: ['owner', 'manager'] },
  { to: '/accounts', label: 'Аккаунты', icon: WalletCards, roles: ['owner', 'manager'] },
  { to: '/employees', label: 'Сотрудники', icon: UsersRound, roles: ['owner'] },
] as const;

const pageTitles: Record<string, string> = {
  '/chats': 'Рабочая зона чатов',
  '/dashboard': 'Состояние отдела',
  '/alerts': 'Очередь алертов',
  '/accounts': 'Telegram-аккаунты',
  '/employees': 'Сотрудники',
};

export function AppShell() {
  const { session } = useDevSession();
  const location = useLocation();
  const accounts = useAccounts(session.companyId);
  const alerts = useAlerts(session.companyId);
  const managerEmployeeId = session.role === 'manager' ? (session.employeeId ?? undefined) : undefined;
  const scopedChats = useChats(
    session.companyId,
    undefined,
    managerEmployeeId,
    session.role === 'manager' && managerEmployeeId !== undefined,
  );
  const company = useCompany(session.companyId);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const isChats = location.pathname === '/chats';
  const visibleNavigation = navigation.filter((item) => (item.roles as readonly string[]).includes(session.role));
  const visibleChatIds = new Set((scopedChats.data ?? []).map((chat) => chat.id));
  const visibleAlertCount = (alerts.data ?? []).filter((alert) => session.role === 'owner' || visibleChatIds.has(alert.chat_id)).length;

  const navigationContent = (
    <>
      <div className="flex h-16 items-center gap-3 border-b border-stone-200 px-4 lg:justify-center 2xl:justify-start">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-blue-600 font-display text-xs font-bold tracking-tight text-white">TC</span>
        <div className="min-w-0 lg:hidden 2xl:block">
          <p className="truncate font-display text-sm font-bold tracking-tight text-stone-950">TeleCorp</p>
          <p className="text-[10px] font-medium uppercase tracking-[0.16em] text-stone-400">Analytics</p>
        </div>
      </div>
      <nav className="flex-1 space-y-1 overflow-y-auto p-3 lg:px-2 2xl:p-3" aria-label="Основная навигация">
        {visibleNavigation.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            onClick={() => setMobileOpen(false)}
            title={item.label}
            aria-label={item.label}
            className={({ isActive }) => cn(
              'flex h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium text-stone-600 transition-colors hover:bg-stone-100 hover:text-stone-950 lg:justify-center lg:px-0 2xl:justify-start 2xl:px-3',
              isActive && 'bg-blue-50 text-blue-700 shadow-[inset_3px_0_0_#2563eb] hover:bg-blue-50 hover:text-blue-700',
            )}
          >
            <item.icon className="size-[18px] shrink-0" aria-hidden="true" />
            <span className="lg:hidden 2xl:inline">{item.label}</span>
            {item.to === '/alerts' && visibleAlertCount > 0 && (
              <span className="ml-auto min-w-5 rounded-full bg-red-100 px-1.5 py-0.5 text-center text-[10px] font-bold tabular-nums text-red-700 lg:absolute lg:ml-5 lg:-translate-y-3 2xl:ml-auto 2xl:translate-y-0">
                {visibleAlertCount}
              </span>
            )}
          </NavLink>
        ))}
      </nav>
      {session.role === 'owner' && (
        <div className="border-t border-stone-200 p-3 lg:p-2 2xl:p-3">
          <button
            className="flex h-10 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left text-sm text-stone-500 hover:bg-stone-100 hover:text-stone-900 lg:justify-center lg:px-0 2xl:justify-start 2xl:px-3"
            onClick={() => setSettingsOpen(true)}
            title="Настройки компании"
          >
            <Settings className="size-[18px]" />
            <span className="lg:hidden 2xl:inline">Настройки</span>
          </button>
        </div>
      )}
      <div className="m-3 rounded-lg border border-stone-200 bg-stone-50 p-3 lg:hidden 2xl:block">
        <p className="flex items-center gap-2 text-[11px] font-medium text-stone-600"><ShieldCheck className="size-3.5" /> Данные в вашем контуре</p>
        <p className="mt-1 text-[10px] leading-4 text-stone-400">Self-hosted рабочее пространство</p>
      </div>
    </>
  );

  return (
    <div className="flex h-dvh overflow-hidden bg-[#f7f7f5] text-stone-800">
      <aside className="relative hidden w-60 shrink-0 flex-col border-r border-stone-200 bg-white md:flex lg:w-16 2xl:w-60">
        {navigationContent}
      </aside>
      {mobileOpen && (
        <div className="fixed inset-0 z-50 bg-stone-950/30 md:hidden" onClick={() => setMobileOpen(false)}>
          <aside className="flex h-full w-72 flex-col bg-white shadow-xl" onClick={(event) => event.stopPropagation()}>
            <button className="absolute right-3 top-4 z-10 rounded-md p-2 text-stone-500" onClick={() => setMobileOpen(false)} aria-label="Закрыть меню"><X className="size-5" /></button>
            {navigationContent}
          </aside>
        </div>
      )}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-16 shrink-0 items-center justify-between gap-3 border-b border-stone-200 bg-white px-4 lg:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <Button variant="ghost" size="icon" className="md:hidden" onClick={() => setMobileOpen(true)} aria-label="Открыть меню"><Menu className="size-5" /></Button>
            <div className="min-w-0">
              <h1 className="truncate font-display text-sm font-semibold text-stone-900 sm:text-base">{pageTitles[location.pathname] ?? 'TeleCorp Analytics'}</h1>
              <p className="hidden truncate text-[11px] text-stone-400 sm:block">{company.data?.name ?? `Компания #${session.companyId}`}</p>
            </div>
          </div>
          <DevSwitcher />
        </header>
        <DisconnectedAccountsBanner
          accounts={accounts.data ?? []}
          isError={accounts.isError}
          error={accounts.error?.message}
          onRetry={() => void accounts.refetch()}
        />
        <main className={cn('min-h-0 min-w-0 flex-1', isChats ? 'overflow-hidden' : 'overflow-y-auto')}>
          <Suspense fallback={<LoadingState label="Загружаем раздел…" className="h-full" />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
      <CompanySettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </div>
  );
}
