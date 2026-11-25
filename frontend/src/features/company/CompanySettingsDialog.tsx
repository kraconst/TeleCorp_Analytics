import { Clock3 } from 'lucide-react';
import { useCompany } from '@/api/hooks';
import { useDevSession } from '@/context/DevSessionContext';
import { Dialog } from '@/components/ui/Dialog';
import { ErrorState, LoadingState } from '@/components/ui/States';

export function CompanySettingsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { session } = useDevSession();
  const company = useCompany(session.companyId);

  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Настройки компании" description="Параметры текущего рабочего пространства">
      <div className="p-5">
        {company.isLoading ? (
          <LoadingState />
        ) : company.isError ? (
          <ErrorState description={company.error.message} onRetry={() => void company.refetch()} />
        ) : company.data ? (
          <div className="space-y-5">
            <dl className="divide-y divide-stone-200 rounded-xl border border-stone-200">
              <div className="grid grid-cols-[140px_1fr] gap-4 px-4 py-3 text-sm">
                <dt className="text-stone-500">Название</dt>
                <dd className="font-medium text-stone-900">{company.data.name}</dd>
              </div>
              <div className="grid grid-cols-[140px_1fr] gap-4 px-4 py-3 text-sm">
                <dt className="text-stone-500">Часовой пояс</dt>
                <dd className="font-medium text-stone-900">{company.data.timezone || 'UTC'}</dd>
              </div>
              <div className="grid grid-cols-[140px_1fr] gap-4 px-4 py-3 text-sm">
                <dt className="text-stone-500">Порог без ответа</dt>
                <dd className="flex items-center gap-2 font-medium tabular-nums text-stone-900">
                  <Clock3 className="size-4 text-stone-500" /> {company.data.noReplyThresholdMinutes} мин
                </dd>
              </div>
            </dl>
            <p className="rounded-lg bg-stone-100 p-3 text-xs leading-5 text-stone-600">
              Просмотр только для чтения. Редактирование станет доступно после появления соответствующего API.
            </p>
          </div>
        ) : null}
      </div>
    </Dialog>
  );
}
