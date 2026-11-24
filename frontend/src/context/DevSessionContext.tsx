import { useQueryClient } from '@tanstack/react-query';
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { DEFAULT_COMPANY_ID, DEV_SESSION_STORAGE_KEY } from '@/config/env';
import type { ActiveUserRole } from '@/types/api';

export interface DevSession {
  companyId: number;
  role: ActiveUserRole;
  employeeId: number | null;
}

interface DevSessionContextValue {
  session: DevSession;
  updateSession: (patch: Partial<DevSession>) => void;
}

const defaultSession: DevSession = { companyId: DEFAULT_COMPANY_ID, role: 'owner', employeeId: null };
const DevSessionContext = createContext<DevSessionContextValue | null>(null);

function loadSession(): DevSession {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(DEV_SESSION_STORAGE_KEY) ?? 'null');
    if (!parsed || typeof parsed !== 'object') return defaultSession;
    const value = parsed as Partial<DevSession>;
    return {
      companyId: Number.isSafeInteger(value.companyId) && (value.companyId ?? 0) > 0 ? value.companyId! : 1,
      role: value.role === 'manager' ? 'manager' : 'owner',
      employeeId: Number.isSafeInteger(value.employeeId) && (value.employeeId ?? 0) > 0 ? value.employeeId! : null,
    };
  } catch {
    return defaultSession;
  }
}

export function DevSessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [session, setSession] = useState<DevSession>(loadSession);

  const updateSession = useCallback(
    (patch: Partial<DevSession>) => {
      void queryClient.cancelQueries();
      queryClient.clear();
      setSession((current) => {
        const next = { ...current, ...patch };
        if (next.role === 'owner') next.employeeId = null;
        try {
          localStorage.setItem(DEV_SESSION_STORAGE_KEY, JSON.stringify(next));
        } catch {
          // The session remains usable when storage is blocked.
        }
        return next;
      });
    },
    [queryClient],
  );

  const value = useMemo(() => ({ session, updateSession }), [session, updateSession]);
  return <DevSessionContext.Provider value={value}>{children}</DevSessionContext.Provider>;
}

export function useDevSession() {
  const context = useContext(DevSessionContext);
  if (!context) throw new Error('useDevSession must be used within DevSessionProvider');
  return context;
}
