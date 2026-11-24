import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiRequest, ApiError, withCompany } from '@/api/client';

afterEach(() => vi.unstubAllGlobals());

describe('API client', () => {
  it('serializes only defined company query params', () => {
    const path = withCompany('/chats', 1, { status: 'new_lead', employee_id: undefined, unanswered: true });
    expect(path).toBe('/chats?company_id=1&status=new_lead&unanswered=true');
  });

  it('returns raw JSON without a data wrapper', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify([{ id: 1 }]), { status: 200 })));
    await expect(apiRequest<{ id: number }[]>('/accounts')).resolves.toEqual([{ id: 1 }]);
  });

  it('parses backend error payloads', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'employee is inactive' }), { status: 400 })));
    const promise = apiRequest('/employees');
    await expect(promise).rejects.toMatchObject({ status: 400, message: 'employee is inactive' });
    await promise.catch((error: unknown) => expect(error).toBeInstanceOf(ApiError));
  });

  it('turns HTML proxy failures into readable errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>Bad gateway</html>', { status: 502 })));
    await expect(apiRequest('/health')).rejects.toMatchObject({ status: 502, message: 'Сервер временно недоступен' });
  });
});
