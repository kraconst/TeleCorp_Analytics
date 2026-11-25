import { AlertTriangle, ArrowRight, RefreshCw } from 'lucide-react';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { displayUsername } from '@/lib/formatters';
import type { Account } from '@/types/api';

export function DisconnectedAccountsBanner({
  accounts,
  isError,
  error,
  onRetry,
}: {
  accounts: Account[];
  isError: boolean;
  error?: string;
  onRetry: () => void;
}) {
  if (isError) {
    return (
      <div className="flex min-h-10 items-center justify-between gap-3 border-b border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-900 lg:px-6">
        <span className="flex items-center gap-2">
          <AlertTriangle className="size-4 shrink-0" aria-hidden="true" />
          Не удалось проверить состояние Telegram-аккаунтов{error ? `: ${error}` : ''}
        </span>
        <Button variant="ghost" size="sm" className="h-7 px-2 text-amber-900" onClick={onRetry}>
          <RefreshCw className="size-4" /> Повторить
        </Button>
      </div>
    );
  }

  const problematic = accounts.filter((account) => account.status === 'disconnected' || account.status === 'banned');
  if (problematic.length === 0) return null;

  return (
    <div className="flex min-h-10 items-center justify-between gap-3 border-b border-red-200 bg-red-50 px-4 py-2 text-xs text-red-900 lg:px-6" role="alert">
      <span className="flex min-w-0 items-center gap-2">
        <AlertTriangle className="size-4 shrink-0 text-red-700" aria-hidden="true" />
        <span className="truncate">
          Сбор сообщений остановлен: {problematic.map((account) => displayUsername(account.username)).join(', ')}
        </span>
      </span>
      <Button asChild variant="ghost" size="sm" className="h-7 px-2 text-red-800 hover:bg-red-100">
        <Link to="/accounts">Проверить аккаунты <ArrowRight className="size-4" /></Link>
      </Button>
    </div>
  );
}
