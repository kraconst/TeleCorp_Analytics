import { describe, expect, it } from 'vitest';
import { alertTypeConfig, chatStatusConfig, getStatusMeta } from '@/lib/statusConfig';

describe('status metadata', () => {
  it('returns the configured Russian label for known values', () => {
    expect(getStatusMeta(chatStatusConfig, 'new_lead', 'Неизвестный статус').label).toBe('Новый лид');
  });

  it('does not throw or expose a raw unknown enum', () => {
    const status = getStatusMeta(chatStatusConfig, 'unexpected-db-value', 'Неизвестный статус');
    expect(status.label).toBe('Неизвестный статус');
    expect(status.className).toContain('stone');
    expect(status.label).not.toContain('unexpected-db-value');
    expect(getStatusMeta(alertTypeConfig, 'unknown-alert', 'Неизвестный алерт').description).toBeTruthy();
  });
});
