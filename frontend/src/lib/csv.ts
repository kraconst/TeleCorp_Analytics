export type CsvValue = string | number | boolean | null | undefined;

function protectFormula(value: string): string {
  return /^[=+\-@\t\r]/.test(value) ? `'${value}` : value;
}

export function csvCell(value: CsvValue): string {
  const normalized = protectFormula(value === null || value === undefined ? '' : String(value));
  return `"${normalized.replaceAll('"', '""')}"`;
}

export function buildCsv(headers: CsvValue[], rows: CsvValue[][]): string {
  return `\uFEFF${[headers, ...rows].map((row) => row.map(csvCell).join(';')).join('\r\n')}`;
}

export function downloadCsv(filename: string, headers: CsvValue[], rows: CsvValue[][]): void {
  const blob = new Blob([buildCsv(headers, rows)], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}
