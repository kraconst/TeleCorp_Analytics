import { describe, expect, it } from 'vitest';
import { buildCsv, csvCell } from '@/lib/csv';

describe('CSV', () => {
  it('uses BOM, semicolons and escaped quotes', () => {
    const csv = buildCsv(['Клиент', 'Текст'], [['ООО "Вектор"', 'Строка 1\nСтрока 2']]);
    expect(csv.charCodeAt(0)).toBe(0xfeff);
    expect(csv).toContain('"ООО ""Вектор""";"Строка 1\nСтрока 2"');
  });

  it.each(['=1+1', '+7999', '-2+3', '@cmd', '\t=cmd'])('neutralizes formula injection in %s', (value) => {
    expect(csvCell(value)).toBe(`"'${value}"`);
  });

  it('keeps empty values explicit', () => {
    expect(buildCsv(['a', 'b'], [[null, undefined]]).endsWith('"";""')).toBe(true);
  });
});
