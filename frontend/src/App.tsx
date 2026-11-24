import { lazy, type ReactNode } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AppShell } from '@/components/layout/AppShell';
import { useDevSession } from '@/context/DevSessionContext';

const AccountsPage = lazy(() => import('@/features/accounts/AccountsPage').then((module) => ({ default: module.AccountsPage })));
const AlertsPage = lazy(() => import('@/features/alerts/AlertsPage').then((module) => ({ default: module.AlertsPage })));
const ChatsWorkspacePage = lazy(() => import('@/features/chats/ChatsWorkspacePage').then((module) => ({ default: module.ChatsWorkspacePage })));
const DashboardPage = lazy(() => import('@/features/dashboard/DashboardPage').then((module) => ({ default: module.DashboardPage })));
const EmployeesPage = lazy(() => import('@/features/employees/EmployeesPage').then((module) => ({ default: module.EmployeesPage })));

function OwnerOnly({ children }: { children: ReactNode }) {
  const { session } = useDevSession();
  return session.role === 'owner' ? children : <Navigate to="/chats" replace />;
}

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/chats" replace />} />
        <Route path="chats" element={<ChatsWorkspacePage />} />
        <Route path="dashboard" element={<OwnerOnly><DashboardPage /></OwnerOnly>} />
        <Route path="alerts" element={<AlertsPage />} />
        <Route path="accounts" element={<AccountsPage />} />
        <Route path="employees" element={<OwnerOnly><EmployeesPage /></OwnerOnly>} />
        <Route path="*" element={<Navigate to="/chats" replace />} />
      </Route>
    </Routes>
  );
}
