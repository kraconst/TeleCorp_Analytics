import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { BarChart3, Clock3, FileText, MessageSquare, Pencil, RefreshCw, Sparkles, UserRound } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { Link } from 'react-router-dom';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { z } from 'zod';
import { generateChatSummary, upsertChatSummary } from '@/api/chats';
import { ApiError } from '@/api/client';
import { useChat, useChatAnalytics } from '@/api/hooks';
import { queryKeys } from '@/api/queryKeys';
import { useToast } from '@/components/feedback/ToastProvider';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input, Textarea } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { ErrorState, LoadingState } from '@/components/ui/States';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/Tabs';
import { contactDisplayName } from '@/lib/chatFilters';
import { displayUsername, formatDateTime, formatDuration, formatNumber } from '@/lib/formatters';
import { getStatusMeta, riskConfig } from '@/lib/statusConfig';
import type { ChatDetails, ChatListItem, ChatSummary } from '@/types/api';

const summarySchema = z.object({
  topic: z.string().max(2000),
  agreements: z.string().max(5000),
  next_step: z.string().max(5000),
  open_questions: z.string().max(5000),
  risk: z.enum(['low', 'medium', 'high']),
  last_message_id: z.number().int().nonnegative(),
});
type SummaryFormValues = z.infer<typeof summarySchema>;

function SummaryForm({ summary, onCancel, onSave, isPending }: {
  summary?: ChatSummary;
  onCancel: () => void;
  onSave: (values: SummaryFormValues) => void;
  isPending: boolean;
}) {
  const { register, handleSubmit } = useForm<SummaryFormValues>({
    resolver: zodResolver(summarySchema),
    defaultValues: {
      topic: summary?.topic ?? '',
      agreements: summary?.agreements ?? '',
      next_step: summary?.next_step ?? '',
      open_questions: summary?.open_questions ?? '',
      risk: summary?.risk ?? 'low',
      last_message_id: summary?.last_message_id ?? 0,
    },
  });
  const fields = [
    { name: 'topic', label: 'Тема' },
    { name: 'agreements', label: 'Договорённости' },
    { name: 'next_step', label: 'Следующий шаг' },
    { name: 'open_questions', label: 'Открытые вопросы' },
  ] as const;
  return (
    <form className="space-y-4" onSubmit={handleSubmit(onSave)}>
      {fields.map((field) => (
        <label key={field.name} className="block text-xs font-medium text-stone-600">
          {field.label}
          <Textarea className="mt-1.5 font-normal" {...register(field.name)} />
        </label>
      ))}
      <label className="block text-xs font-medium text-stone-600">Риск
        <Select className="mt-1.5 w-full" {...register('risk')}>
          <option value="low">Низкий</option><option value="medium">Средний</option><option value="high">Высокий</option>
        </Select>
      </label>
      <Input type="hidden" {...register('last_message_id', { valueAsNumber: true })} />
      <div className="flex justify-end gap-2 border-t border-stone-200 pt-4">
        <Button variant="ghost" onClick={onCancel}>Отмена</Button>
        <Button type="submit" disabled={isPending}>{isPending ? 'Сохраняем…' : 'Сохранить'}</Button>
      </div>
    </form>
  );
}

function SummaryTab({ companyId, chat }: { companyId: number; chat: ChatDetails }) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const [editing, setEditing] = useState(false);
  const [generationError, setGenerationError] = useState<string | null>(null);
  const summary = chat.summary;
  const invalidate = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.chat(companyId, chat.id) }),
    queryClient.invalidateQueries({ queryKey: queryKeys.summary(companyId, chat.id) }),
  ]);
  const saveMutation = useMutation({
    mutationFn: (values: SummaryFormValues) => upsertChatSummary(companyId, chat.id, values),
    onSuccess: async () => {
      await invalidate();
      setEditing(false);
      showToast('Резюме сохранено');
    },
    onError: (error) => showToast(error.message, 'error'),
  });
  const generateMutation = useMutation({
    mutationFn: () => generateChatSummary(companyId, chat.id),
    onMutate: () => setGenerationError(null),
    onSuccess: (generated) => {
      queryClient.setQueryData<ChatDetails>(queryKeys.chat(companyId, chat.id), (old) => old ? { ...old, summary: generated } : old);
      queryClient.setQueryData(queryKeys.summary(companyId, chat.id), generated);
      showToast('AI-саммари обновлено');
    },
    onError: (error) => {
      setGenerationError(error instanceof ApiError && (error.status === 404 || error.status === 503 || error.isTimeout)
        ? 'AI-генерация сейчас недоступна на сервере. Существующее саммари сохранено; его можно дополнить вручную.'
        : error.message);
    },
  });

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-xs text-stone-400"><Sparkles className="size-3.5" /> Контекст диалога</div>
        <div className="flex gap-1">
          {summary && !editing && <Button variant="ghost" size="sm" onClick={() => setEditing(true)}><Pencil className="size-3.5" /> Изменить</Button>}
          <Button variant="ghost" size="sm" onClick={() => generateMutation.mutate()} disabled={generateMutation.isPending}>
            <RefreshCw className={`size-3.5 ${generateMutation.isPending ? 'animate-spin' : ''}`} />
            {generateMutation.isPending ? 'Генерируем…' : summary ? 'Обновить AI' : 'Сгенерировать'}
          </Button>
        </div>
      </div>
      {generationError && <ErrorState className="mb-4" title="Генерация не выполнена" description={generationError} onRetry={() => generateMutation.mutate()} />}
      {generateMutation.isPending && <LoadingState label="Сервер формирует саммари…" className="mb-4" />}
      {editing ? (
        <SummaryForm key={`${chat.id}-${summary?.generated_at ?? 'new'}`} summary={summary} onCancel={() => setEditing(false)} onSave={(values) => saveMutation.mutate(values)} isPending={saveMutation.isPending} />
      ) : summary ? (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Badge className={getStatusMeta(riskConfig, summary.risk, 'Неизвестный риск').className}>{getStatusMeta(riskConfig, summary.risk, 'Неизвестный риск').label}</Badge>
            <span className="flex items-center gap-1 text-[10px] tabular-nums text-stone-400"><Clock3 className="size-3" /> {formatDateTime(summary.generated_at)}</span>
          </div>
          {[
            ['Тема', summary.topic],
            ['Договорённости', summary.agreements],
            ['Следующий шаг', summary.next_step],
            ['Открытые вопросы', summary.open_questions],
          ].map(([label, value], index) => (
            <section key={label} className={index === 2 ? 'rounded-lg border border-blue-200 bg-blue-50/60 p-3' : 'border-l-2 border-stone-200 pl-3'}>
              <h4 className="text-[10px] font-semibold uppercase tracking-[0.12em] text-stone-400">{label}</h4>
              <p className="mt-1.5 whitespace-pre-wrap break-words text-sm leading-5 text-stone-700">{value || 'Не заполнено'}</p>
            </section>
          ))}
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-stone-300 bg-stone-50 p-5 text-center">
          <FileText className="mx-auto mb-3 size-5 text-stone-400" />
          <h3 className="text-sm font-semibold text-stone-800">Саммари ещё не сформировано</h3>
          <p className="mt-1 text-xs leading-5 text-stone-500">Запустите генерацию на сервере или зафиксируйте контекст вручную.</p>
          <Button variant="secondary" size="sm" className="mt-4" onClick={() => setEditing(true)}><Pencil className="size-3.5" /> Заполнить вручную</Button>
        </div>
      )}
    </div>
  );
}

function ContactAnalyticsTab({ companyId, chat, allChats }: { companyId: number; chat: ChatDetails; allChats: ChatListItem[] }) {
  const analytics = useChatAnalytics(companyId, chat.id);
  const parallelChats = allChats.filter((item) => item.contact_id === chat.contact_id && item.id !== chat.id);
  const contact = chat.contact;
  return (
    <div className="space-y-5">
      <section>
        <h3 className="mb-3 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-stone-400"><UserRound className="size-3.5" /> Контакт</h3>
        <div className="rounded-lg border border-stone-200 p-3">
          <p className="text-sm font-semibold text-stone-900">{contact?.name || `Контакт #${chat.contact_id}`}</p>
          <dl className="mt-3 space-y-2 text-xs">
            <div className="flex justify-between gap-3"><dt className="text-stone-400">Username</dt><dd className="font-medium text-stone-700">{displayUsername(contact?.username)}</dd></div>
            <div className="flex justify-between gap-3"><dt className="text-stone-400">Telegram ID</dt><dd className="font-medium tabular-nums text-stone-700">{contact?.telegram_user_id ?? '—'}</dd></div>
          </dl>
        </div>
      </section>
      {parallelChats.length > 0 && (
        <section className="rounded-lg border border-amber-200 bg-amber-50 p-3">
          <h3 className="text-xs font-semibold text-amber-900">Другие чаты этого клиента</h3>
          <div className="mt-2 space-y-1.5">
            {parallelChats.map((item) => <Link key={item.id} className="block truncate text-xs text-amber-800 underline decoration-amber-300 underline-offset-2" to={`/chats?view=split&chat_id=${item.id}`}>Чат #{item.id} · {item.employee?.name || item.account?.employee_name || displayUsername(item.account?.username)}</Link>)}
          </div>
        </section>
      )}
      <section>
        <h3 className="mb-3 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-stone-400"><BarChart3 className="size-3.5" /> Аналитика за всю историю</h3>
        {analytics.isLoading ? <LoadingState /> : analytics.isError ? <ErrorState description={analytics.error.message} onRetry={() => void analytics.refetch()} /> : analytics.data ? (
          <>
            <div className="grid grid-cols-3 gap-2">
              {[
                ['Входящие', analytics.data.messages_in],
                ['Исходящие', analytics.data.messages_out],
                ['Медиа', analytics.data.media_messages],
              ].map(([label, value]) => <div key={label} className="rounded-lg bg-stone-100 p-2.5 text-center"><p className="text-lg font-semibold tabular-nums text-stone-900">{formatNumber(Number(value))}</p><p className="mt-0.5 text-[10px] text-stone-500">{label}</p></div>)}
            </div>
            <dl className="mt-3 divide-y divide-stone-100 rounded-lg border border-stone-200 px-3 text-xs">
              <div className="flex justify-between py-2.5"><dt className="text-stone-500">Среднее время ответа</dt><dd className="font-medium tabular-nums text-stone-800">{formatDuration(analytics.data.avg_response_seconds)}</dd></div>
              <div className="flex justify-between py-2.5"><dt className="text-stone-500">Максимум</dt><dd className="font-medium tabular-nums text-stone-800">{formatDuration(analytics.data.max_response_seconds)}</dd></div>
              <div className="flex justify-between py-2.5"><dt className="text-stone-500">Серий с ответом</dt><dd className="font-medium tabular-nums text-stone-800">{analytics.data.response_count}</dd></div>
            </dl>
            <div className="mt-4">
              <p className="mb-2 text-[11px] font-medium text-stone-500">Активность по дням · UTC</p>
              {analytics.data.daily_activity.length ? (
                <div className="h-44 w-full" aria-label="График входящих и исходящих сообщений по дням">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={analytics.data.daily_activity} margin={{ top: 5, right: 0, bottom: 0, left: -25 }}>
                      <CartesianGrid vertical={false} stroke="#e7e5e4" />
                      <XAxis dataKey="day" tick={{ fontSize: 9, fill: '#78716c' }} tickFormatter={(value: string) => value.slice(5)} axisLine={false} tickLine={false} />
                      <YAxis allowDecimals={false} tick={{ fontSize: 9, fill: '#78716c' }} axisLine={false} tickLine={false} />
                      <Tooltip contentStyle={{ fontSize: 12, borderRadius: 8, border: '1px solid #e7e5e4' }} />
                      <Bar dataKey="messages_in" name="Входящие" fill="#93c5fd" radius={[3, 3, 0, 0]} />
                      <Bar dataKey="messages_out" name="Исходящие" fill="#2563eb" radius={[3, 3, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              ) : <p className="rounded-lg bg-stone-50 p-4 text-center text-xs text-stone-400">Пока нет активности для графика</p>}
            </div>
          </>
        ) : null}
      </section>
      <p className="flex gap-2 text-[10px] leading-4 text-stone-400"><MessageSquare className="mt-0.5 size-3 shrink-0" /> Метрики рассчитываются только по сохранённой истории, без обращения к Telegram.</p>
    </div>
  );
}

export function ChatRightPanel({ companyId, chatId, allChats }: { companyId: number; chatId?: number; allChats: ChatListItem[] }) {
  const chatQuery = useChat(companyId, chatId);
  if (!chatId) return <div className="p-5 text-sm text-stone-400">Выберите чат, чтобы увидеть детали.</div>;
  if (chatQuery.isLoading) return <LoadingState className="h-full" />;
  if (chatQuery.isError || !chatQuery.data) return <ErrorState className="m-4" description={chatQuery.error?.message ?? 'Чат не найден'} onRetry={() => void chatQuery.refetch()} />;
  const selectedName = chatQuery.data.contact?.name || allChats.find((item) => item.id === chatId)?.contact?.name || contactDisplayName({ contact: null, contact_id: chatQuery.data.contact_id });
  return (
    <Tabs defaultValue="summary" className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 p-3 pb-0"><TabsList><TabsTrigger value="summary">Саммари</TabsTrigger><TabsTrigger value="analytics">Контакт и аналитика</TabsTrigger></TabsList></div>
      <div className="sr-only" aria-live="polite">Детали чата: {selectedName}</div>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <TabsContent value="summary"><SummaryTab companyId={companyId} chat={chatQuery.data} /></TabsContent>
        <TabsContent value="analytics"><ContactAnalyticsTab companyId={companyId} chat={chatQuery.data} allChats={allChats} /></TabsContent>
      </div>
    </Tabs>
  );
}
