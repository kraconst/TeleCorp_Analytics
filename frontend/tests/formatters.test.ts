import { describe, expect, it } from 'vitest';
import { formatDuration } from '@/lib/formatters';

describe('formatDuration', () => {
  it.each([
    [null, '—'],
    [undefined, '—'],
    [0, '0 сек'],
    [12, '12 сек'],
    [185.5, '3 мин 6 сек'],
    [240, '4 мин'],
    [3600, '1 ч'],
    [3720, '1 ч 2 мин'],
    [90_000, '1 д 1 ч'],
  ])('formats %s as %s', (input, expected) => {
    expect(formatDuration(input)).toBe(expected);
  });
});
