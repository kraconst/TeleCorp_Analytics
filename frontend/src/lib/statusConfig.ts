import type {
  AccountStatus,
  AlertType,
  ChatPriority,
  ChatStatus,
  RiskLevel,
  StatusSource,
} from '@/types/api';

interface StatusMeta {
  label: string;
  className: string;
  description?: string;
}

const unknownStatusMeta = (label: string): StatusMeta => ({
  label,
  className: 'border-stone-300 bg-stone-100 text-stone-600',
  description: 'Значение не распознано. Проверьте формат данных сервера.',
});

export function getStatusMeta<T extends string>(
  config: Record<T, StatusMeta>,
  value: string,
  fallbackLabel: string,
): StatusMeta {
  return Object.hasOwn(config, value) ? config[value as T] : unknownStatusMeta(fallbackLabel);
}

export const chatStatusOrder: ChatStatus[] = [
  'new_lead',
  'negotiation',
  'waiting_reply',
  'action_required',
  'interested',
  'unhappy',
  'lost',
];

export const chatStatusConfig: Record<ChatStatus, StatusMeta> = {
  new_lead: { label: 'Новый лид', className: 'border-sky-200 bg-sky-50 text-sky-700' },
  negotiation: { label: 'Переговоры', className: 'border-blue-200 bg-blue-50 text-blue-700' },
  waiting_reply: { label: 'Ждёт ответа', className: 'border-amber-200 bg-amber-50 text-amber-800' },
  action_required: {
    label: 'Требует внимания',
    className: 'border-red-200 bg-red-50 text-red-700',
  },
  interested: {
    label: 'Заинтересован',
    className: 'border-emerald-200 bg-emerald-50 text-emerald-700',
  },
  unhappy: { label: 'Негатив', className: 'border-orange-200 bg-orange-50 text-orange-700' },
  lost: { label: 'Потерян', className: 'border-stone-300 bg-stone-100 text-stone-600' },
};

export const priorityConfig: Record<ChatPriority, StatusMeta> = {
  low: { label: 'Низкий', className: 'border-stone-200 bg-stone-100 text-stone-600' },
  normal: { label: 'Обычный', className: 'border-amber-200 bg-amber-50 text-amber-800' },
  high: { label: 'Высокий', className: 'border-red-200 bg-red-50 text-red-700' },
};

export const riskConfig: Record<RiskLevel, StatusMeta> = {
  low: { label: 'Низкий риск', className: 'border-stone-200 bg-stone-100 text-stone-600' },
  medium: { label: 'Средний риск', className: 'border-amber-200 bg-amber-50 text-amber-800' },
  high: { label: 'Высокий риск', className: 'border-red-200 bg-red-50 text-red-700' },
};

export const accountStatusConfig: Record<AccountStatus, StatusMeta> = {
  active: { label: 'Активен', className: 'border-emerald-200 bg-emerald-50 text-emerald-700' },
  connecting: { label: 'Подключается', className: 'border-blue-200 bg-blue-50 text-blue-700' },
  disconnected: { label: 'Отключён', className: 'border-red-200 bg-red-50 text-red-700' },
  banned: { label: 'Заблокирован', className: 'border-rose-200 bg-rose-100 text-rose-900' },
};

export const alertTypeConfig: Record<AlertType, StatusMeta> = {
  no_reply: {
    label: 'Нет ответа',
    className: 'border-amber-200 bg-amber-50 text-amber-800',
    description: 'Клиент ждёт ответа дольше допустимого времени',
  },
  complaint: {
    label: 'Жалоба',
    className: 'border-red-200 bg-red-50 text-red-700',
    description: 'В диалоге зафиксирована жалоба клиента',
  },
  refusal: {
    label: 'Отказ',
    className: 'border-stone-200 bg-stone-100 text-stone-700',
    description: 'В диалоге зафиксирован отказ от предложения',
  },
  negative: {
    label: 'Негатив',
    className: 'border-orange-200 bg-orange-50 text-orange-800',
    description: 'В диалоге зафиксирована негативная реакция',
  },
  urgent: {
    label: 'Срочно',
    className: 'border-rose-200 bg-rose-100 text-rose-800',
    description: 'Запрос клиента отмечен как срочный',
  },
};

export const statusSourceConfig: Record<StatusSource, string> = {
  auto: 'Автоматический статус',
  manual: 'Изменён вручную',
};

export function isChatStatus(value: string | null): value is ChatStatus {
  return value !== null && chatStatusOrder.includes(value as ChatStatus);
}

export function isAlertType(value: string | null): value is AlertType {
  return value !== null && Object.hasOwn(alertTypeConfig, value);
}
