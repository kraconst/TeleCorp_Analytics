import { AlertCircle, LoaderCircle, SearchX, type LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Button } from './Button';

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-stone-200/80', className)} aria-hidden="true" />;
}

export function LoadingState({ label = 'Загружаем данные…', className }: { label?: string; className?: string }) {
  return (
    <div className={cn('flex items-center justify-center gap-2 p-6 text-sm text-stone-500', className)} role="status">
      <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
      {label}
    </div>
  );
}

interface StatePanelProps {
  title: string;
  description: string;
  icon?: LucideIcon;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, icon: Icon = SearchX, action, className }: StatePanelProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center px-6 py-10 text-center', className)}>
      <span className="mb-3 rounded-lg border border-stone-200 bg-stone-50 p-3 text-stone-500">
        <Icon className="size-[18px]" aria-hidden="true" />
      </span>
      <h3 className="font-display text-sm font-medium text-stone-800">{title}</h3>
      <p className="mt-1 max-w-sm text-sm leading-5 text-stone-500">{description}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorState({
  title = 'Не удалось загрузить данные',
  description,
  onRetry,
  className,
}: {
  title?: string;
  description: string;
  onRetry?: () => void;
  className?: string;
}) {
  return (
    <div className={cn('rounded-xl border border-red-200 bg-red-50 p-4 text-red-800', className)} role="alert">
      <div className="flex items-start gap-3">
        <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{title}</p>
          <p className="mt-0.5 break-words text-xs leading-5 text-red-700">{description}</p>
          {onRetry && (
            <Button variant="danger" size="sm" className="mt-3" onClick={onRetry}>
              Повторить
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
