import { X } from 'lucide-react';
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';

interface ToastItem {
  id: number;
  message: string;
  tone: 'success' | 'error';
}

interface ToastContextValue {
  showToast: (message: string, tone?: ToastItem['tone']) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const dismiss = useCallback((id: number) => setToasts((items) => items.filter((item) => item.id !== id)), []);
  const showToast = useCallback(
    (message: string, tone: ToastItem['tone'] = 'success') => {
      const id = Date.now() + Math.random();
      setToasts((items) => [...items.slice(-2), { id, message, tone }]);
      window.setTimeout(() => dismiss(id), 4500);
    },
    [dismiss],
  );
  const value = useMemo(() => ({ showToast }), [showToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-[min(360px,calc(100vw-2rem))] flex-col gap-2" aria-live="polite">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`pointer-events-auto flex items-start gap-3 rounded-lg border bg-white p-3 text-sm shadow-lg ${
              toast.tone === 'error' ? 'border-red-200 text-red-800' : 'border-emerald-200 text-emerald-900'
            }`}
          >
            <span className="min-w-0 flex-1 leading-5">{toast.message}</span>
            <button className="cursor-pointer text-stone-400 hover:text-stone-700" onClick={() => dismiss(toast.id)} aria-label="Закрыть уведомление">
              <X className="size-4" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error('useToast must be used within ToastProvider');
  return context;
}
