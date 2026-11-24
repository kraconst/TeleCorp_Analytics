import { formatDistanceStrict, parseISO } from 'date-fns';
import { ru } from 'date-fns/locale';

const numberFormatter = new Intl.NumberFormat('ru-RU');

export function formatNumber(value: number): string {
  return numberFormatter.format(value);
}

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds) || seconds < 0) return '—';
  const rounded = Math.round(seconds);
  if (rounded < 60) return `${rounded} сек`;
  const minutes = Math.floor(rounded / 60);
  const restSeconds = rounded % 60;
  if (minutes < 60) return restSeconds ? `${minutes} мин ${restSeconds} сек` : `${minutes} мин`;
  const hours = Math.floor(minutes / 60);
  const restMinutes = minutes % 60;
  if (hours < 24) return restMinutes ? `${hours} ч ${restMinutes} мин` : `${hours} ч`;
  const days = Math.floor(hours / 24);
  const restHours = hours % 24;
  return restHours ? `${days} д ${restHours} ч` : `${days} д`;
}

export function parseDate(value: string | null | undefined): Date | null {
  if (!value) return null;
  const date = parseISO(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

export function formatDateTime(value: string | null | undefined, timeZone?: string): string {
  const date = parseDate(value);
  if (!date) return '—';
  try {
    return new Intl.DateTimeFormat('ru-RU', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: timeZone || undefined,
    }).format(date);
  } catch {
    return new Intl.DateTimeFormat('ru-RU', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
    }).format(date);
  }
}

export function formatMessageTime(value: string): string {
  const date = parseDate(value);
  return date ? new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' }).format(date) : '';
}

export function formatDay(value: string): string {
  const date = parseDate(value);
  return date ? new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' }).format(date) : value;
}

export function formatRelativeTime(value: string | null | undefined, now = new Date()): string {
  const date = parseDate(value);
  if (!date) return '—';
  return formatDistanceStrict(date, now, { locale: ru, addSuffix: true });
}

export function formatWaitingTime(value: string | null | undefined, now = new Date()): string {
  const date = parseDate(value);
  if (!date) return '—';
  const seconds = Math.max(0, (now.getTime() - date.getTime()) / 1000);
  return formatDuration(seconds);
}

export function toDateInputValue(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function getDateRange(days: number, now = new Date()) {
  const to = new Date(now);
  const from = new Date(now);
  from.setDate(from.getDate() - days);
  return { from: toDateInputValue(from), to: toDateInputValue(to) };
}

export function displayUsername(value: string | null | undefined): string {
  if (!value) return '—';
  return value.startsWith('@') ? value : `@${value}`;
}
