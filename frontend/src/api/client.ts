import type { ApiRequestOptions } from '@/types/api';

const API_PREFIX = '/api';
const DEFAULT_TIMEOUT = 15_000;

export class ApiError extends Error {
  readonly status: number;
  readonly isTimeout: boolean;

  constructor(status: number, message: string, isTimeout = false) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.isTimeout = isTimeout;
  }
}

export function withCompany(
  path: string,
  companyId: number,
  params: Record<string, string | number | boolean | undefined> = {},
): string {
  const search = new URLSearchParams();
  search.set('company_id', String(companyId));
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== '') search.set(key, String(value));
  });
  return `${path}?${search.toString()}`;
}

function errorMessage(payload: unknown, status: number): string {
  if (payload && typeof payload === 'object' && 'error' in payload) {
    const message = (payload as { error?: unknown }).error;
    if (typeof message === 'string' && message.trim()) return message;
  }
  if (typeof payload === 'string' && payload.trim() && !payload.trim().startsWith('<')) {
    return payload.slice(0, 300);
  }
  if (status === 404) return 'Запрошенные данные не найдены';
  if (status === 403) return 'Действие запрещено сервером';
  if (status >= 500) return 'Сервер временно недоступен';
  return `Ошибка запроса (${status})`;
}

export async function apiRequest<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const { body, timeoutMs = DEFAULT_TIMEOUT, signal: externalSignal, headers, ...init } = options;
  const controller = new AbortController();
  let didTimeout = false;
  const timeout = window.setTimeout(() => {
    didTimeout = true;
    controller.abort();
  }, timeoutMs);
  const abortFromParent = () => controller.abort();
  externalSignal?.addEventListener('abort', abortFromParent, { once: true });

  try {
    const response = await fetch(`${API_PREFIX}${path}`, {
      ...init,
      headers: {
        Accept: 'application/json',
        ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
        ...headers,
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal: controller.signal,
    });
    const text = await response.text();
    let payload: unknown = null;
    if (text) {
      try {
        payload = JSON.parse(text);
      } catch {
        payload = text;
      }
    }
    if (!response.ok) throw new ApiError(response.status, errorMessage(payload, response.status));
    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (externalSignal?.aborted) throw new ApiError(0, 'Запрос отменён');
    if (didTimeout) throw new ApiError(408, 'Сервер не ответил вовремя', true);
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError(0, 'Запрос отменён');
    }
    throw new ApiError(0, 'Не удалось связаться с сервером. Проверьте подключение и Vite proxy.');
  } finally {
    window.clearTimeout(timeout);
    externalSignal?.removeEventListener('abort', abortFromParent);
  }
}
