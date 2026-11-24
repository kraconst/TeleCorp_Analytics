import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import type { Alert } from '@/types/api';

export interface SessionResolvedAlert {
  alert: Alert;
  resolvedAt: string;
}

interface ResolvedAlertsContextValue {
  resolved: SessionResolvedAlert[];
  addResolved: (alert: Alert) => void;
}

const ResolvedAlertsContext = createContext<ResolvedAlertsContextValue | null>(null);

export function ResolvedAlertsProvider({ children }: { children: ReactNode }) {
  const [resolved, setResolved] = useState<SessionResolvedAlert[]>([]);
  const addResolved = useCallback((alert: Alert) => {
    setResolved((items) => [
      { alert: { ...alert, resolved_at: new Date().toISOString() }, resolvedAt: new Date().toISOString() },
      ...items.filter((item) => item.alert.id !== alert.id),
    ]);
  }, []);
  const value = useMemo(() => ({ resolved, addResolved }), [resolved, addResolved]);
  return <ResolvedAlertsContext.Provider value={value}>{children}</ResolvedAlertsContext.Provider>;
}

export function useResolvedAlerts() {
  const context = useContext(ResolvedAlertsContext);
  if (!context) throw new Error('useResolvedAlerts must be used within ResolvedAlertsProvider');
  return context;
}
