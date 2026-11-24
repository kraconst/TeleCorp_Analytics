import { createContext, useContext, type ReactNode } from 'react';
import { useNow } from '@/hooks/useNow';

const NowContext = createContext<Date | null>(null);

export function NowProvider({ children }: { children: ReactNode }) {
  const now = useNow(30_000);
  return <NowContext.Provider value={now}>{children}</NowContext.Provider>;
}

export function useSharedNow() {
  return useContext(NowContext) ?? new Date();
}
